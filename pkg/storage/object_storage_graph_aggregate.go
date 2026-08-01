package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/logging"
)

// Aggregate performs aggregations on objects matching the filter using Cypher
//
//nolint:gocyclo // Function orchestrates query building and execution; complexity reduced via helper methods
func (g *GraphObjectStorage) Aggregate(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter, aggregations []Aggregation) (*AggregateResult, error) { //nolint:gocritic // Interface requires value semantics for ListFilter
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	// Check permission
	if err := g.checkPermission(secCtx, "read", filter.Kind); err != nil {
		return nil, err
	}

	// Validate aggregations
	if err := g.validateAggregations(aggregations); err != nil {
		return nil, err
	}

	// Build Cypher query
	query, params, err := g.buildAggregationQuery(&filter, aggregations)
	if err != nil {
		return nil, err
	}

	// Execute query
	result, err := g.executeAggregationQuery(ctx, query, params)
	if err != nil {
		return nil, err
	}

	// Process results
	return g.processAggregationResults(result, aggregations, filter.GroupBy != emptyValue)
}

// validateAggregations validates that at least one aggregation is provided
func (g *GraphObjectStorage) validateAggregations(aggregations []Aggregation) error {
	if len(aggregations) == 0 {
		return errfmt.Errorf(ConstStreamAtLeastOneAggregationIsRequired)
	}
	return nil
}

// buildAggregationQuery builds the Cypher query for aggregation
func (g *GraphObjectStorage) buildAggregationQuery(filter *ListFilter, aggregations []Aggregation) (query string, params map[string]any, err error) {
	label := toLabel(filter.Kind)
	query = fmt.Sprintf(ConstStreamMatchNStrEntity, label)

	// Build parameters map
	params = make(map[string]any)

	// Add filters
	query = g.addFiltersToAggregationQuery(query, filter, params)

	// Build aggregation expressions
	aggExpressions, err := g.buildAggregationExpressions(aggregations)
	if err != nil {
		return "", nil, err
	}

	// Add GROUP BY if specified
	query = g.addGroupByToQuery(query, filter.GroupBy, aggExpressions)

	return query, params, nil
}

// addFiltersToAggregationQuery adds filter conditions to the query
func (g *GraphObjectStorage) addFiltersToAggregationQuery(query string, filter *ListFilter, params map[string]any) string {
	if len(filter.Filters) == 0 {
		return query
	}

	conditions := []string{}
	for field, filterValue := range filter.Filters {
		if filterMap, ok := filterValue.(map[string]any); ok {
			for opStr, opValue := range filterMap {
				operator := FilterOperator(opStr)
				cleanOp := strings.ReplaceAll(opStr, "$", "")
				paramName := fmt.Sprintf("filter_%s_%s", field, cleanOp)
				condition := g.buildCypherCondition(field, operator, paramName)
				if condition != emptyValue {
					conditions = append(conditions, condition)
					params[paramName] = opValue
				}
			}
		} else {
			paramName := fmt.Sprintf("filter_%s", field)
			conditions = append(conditions, fmt.Sprintf("n.%s = $%s", field, paramName))
			params[paramName] = filterValue
		}
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	return query
}

// buildAggregationExpressions builds aggregation expressions from aggregations
func (g *GraphObjectStorage) buildAggregationExpressions(aggregations []Aggregation) ([]string, error) {
	aggExpressions := []string{}

	for _, agg := range aggregations {
		alias := g.getAggregationAlias(agg)
		expr, err := g.buildAggregationExpression(agg)
		if err != nil {
			return nil, err
		}

		aggExpressions = append(aggExpressions, fmt.Sprintf("%s AS %s", expr, alias))
	}

	return aggExpressions, nil
}

// getAggregationAlias gets the alias for an aggregation
func (g *GraphObjectStorage) getAggregationAlias(agg Aggregation) string {
	if agg.Alias != emptyValue {
		return agg.Alias
	}
	if agg.Field != emptyValue {
		return fmt.Sprintf("%s_%s", agg.Function, agg.Field)
	}
	return string(agg.Function)
}

// buildAggregationExpression builds a single aggregation expression
func (g *GraphObjectStorage) buildAggregationExpression(agg Aggregation) (string, error) {
	switch agg.Function {
	case AggregationCount:
		if agg.Field == emptyValue {
			return "COUNT(n)", nil
		}
		return fmt.Sprintf("COUNT(n.%s)", agg.Field), nil

	case AggregationSum:
		if agg.Field == emptyValue {
			return "", errfmt.Errorf(ConstStreamSumAggregationRequiresAField)
		}
		return fmt.Sprintf("SUM(n.%s)", agg.Field), nil

	case AggregationAvg:
		if agg.Field == emptyValue {
			return "", errfmt.Errorf(ConstStreamAvgAggregationRequiresAField)
		}
		return fmt.Sprintf("AVG(n.%s)", agg.Field), nil

	case AggregationMin:
		if agg.Field == emptyValue {
			return "", errfmt.Errorf(ConstStreamMinAggregationRequiresAField)
		}
		return fmt.Sprintf("MIN(n.%s)", agg.Field), nil

	case AggregationMax:
		if agg.Field == emptyValue {
			return "", errfmt.Errorf(ConstStreamMaxAggregationRequiresAField)
		}
		return fmt.Sprintf("MAX(n.%s)", agg.Field), nil

	default:
		return "", errfmt.Errorf(ConstStreamUnsupportedAggregationFunctionStr, agg.Function)
	}
}

// addGroupByToQuery adds GROUP BY clause to query if needed
func (g *GraphObjectStorage) addGroupByToQuery(query, groupBy string, aggExpressions []string) string {
	if groupBy != emptyValue {
		query += fmt.Sprintf(ConstStreamWithNStrAsGroupKeyStr, groupBy, strings.Join(aggExpressions, ", "))
		query += fmt.Sprintf(ConstStreamReturnGroupKeyStr, strings.Join(aggExpressions, ", "))
	} else {
		query += " RETURN " + strings.Join(aggExpressions, ", ")
	}
	return query
}

// executeAggregationQuery executes the aggregation query
func (g *GraphObjectStorage) executeAggregationQuery(ctx context.Context, query string, params map[string]any) (*provider.QueryResult, error) {
	result, err := g.conn.ExecuteQuery(ctx, provider.Query{
		Language: provider.QueryLanguageCypher,
		Query:    query,
		Params:   params,
	})
	if err != nil {
		if isGraphRetryable(err) {
			logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf("Transient error in GraphObjectStorage.executeAggregationQuery: %v", err), nil).Log()
		}
		return nil, errfmt.Newf(ConstStreamFailedToExecuteAggregationQuery).Wrap(err)
	}
	return result, nil
}

// processAggregationResults processes aggregation results based on grouping
func (g *GraphObjectStorage) processAggregationResults(result *provider.QueryResult, aggregations []Aggregation, isGrouped bool) (*AggregateResult, error) {
	if isGrouped {
		return g.processGroupedAggregationResult(result, aggregations)
	}
	return g.processAggregationResult(result, aggregations)
}

// processAggregationResult processes a single-row aggregation result
func (g *GraphObjectStorage) processAggregationResult(result *provider.QueryResult, aggregations []Aggregation) (*AggregateResult, error) {
	aggResult := &AggregateResult{
		Aggregations: make(map[string]any),
		Groups:       nil,
		Meta:         make(map[string]any),
	}

	if len(result.Rows) == 0 {
		// No results - return zero values
		for _, agg := range aggregations {
			alias := agg.Alias
			if alias == emptyValue {
				if agg.Field != emptyValue {
					alias = fmt.Sprintf("%s_%s", agg.Function, agg.Field)
				} else {
					alias = string(agg.Function)
				}
			}
			aggResult.Aggregations[alias] = 0
		}
		return aggResult, nil
	}

	// First row contains aggregation results
	// Rows are maps with column aliases as keys
	row := result.Rows[0]
	for _, agg := range aggregations {
		alias := agg.Alias
		if alias == emptyValue {
			if agg.Field != emptyValue {
				alias = fmt.Sprintf("%s_%s", agg.Function, agg.Field)
			} else {
				alias = string(agg.Function)
			}
		}

		// Get value from row map using alias as key
		if value, ok := row[alias]; ok {
			aggResult.Aggregations[alias] = value
		}
	}

	aggResult.Meta["row_count"] = len(result.Rows)
	return aggResult, nil
}

// processGroupedAggregationResult processes a multi-row grouped aggregation result
func (g *GraphObjectStorage) processGroupedAggregationResult(result *provider.QueryResult, aggregations []Aggregation) (*AggregateResult, error) {
	aggResult := &AggregateResult{
		Aggregations: make(map[string]any),
		Groups:       make(map[string]*AggregateResult),
		Meta:         make(map[string]any),
	}

	// Each row represents a group
	// Rows are maps with column aliases as keys: {"group_key": value, "alias1": value, "alias2": value, ...}
	for _, row := range result.Rows {
		if len(row) == 0 {
			continue
		}

		// Get group key from row map
		groupKeyValue, ok := row["group_key"]
		if !ok {
			// Fallback: try to find first non-aggregation value
			continue
		}
		groupKey := fmt.Sprintf("%v", groupKeyValue)

		// Remaining columns are aggregations
		groupResult := &AggregateResult{
			Aggregations: make(map[string]any),
			Groups:       nil,
			Meta:         make(map[string]any),
		}

		for _, agg := range aggregations {
			alias := agg.Alias
			if alias == emptyValue {
				if agg.Field != emptyValue {
					alias = fmt.Sprintf("%s_%s", agg.Function, agg.Field)
				} else {
					alias = string(agg.Function)
				}
			}

			// Get value from row map using alias as key
			if value, ok := row[alias]; ok {
				groupResult.Aggregations[alias] = value
			}
		}

		aggResult.Groups[groupKey] = groupResult
	}

	aggResult.Meta["group_count"] = len(aggResult.Groups)
	return aggResult, nil
}
