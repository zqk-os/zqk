package cli

// This file contains example usage of CommandBuilder to demonstrate the pattern
// These examples show how to refactor existing commands to use the builder

/*
Example 1: Simple Get Command

Before:
```go
func NewGetCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Get an object by ID",
		`Get an object by its ID.`,
	).
		AddExample("Get a backlog item", "%s get ITEM-626").
		ExcludeCommonFlags()

	getCmd := &cobra.Command{
		Use:  "get <id>",
		Args: cobra.ExactArgs(1),
		RunE: runGet,
	}
	helpBuilder.ApplyToCommand(getCmd)
	cli.AddCommonFlags(getCmd)
	return getCmd
}
```

After (using CommandBuilder):
```go
func NewGetCmd() *cobra.Command {
	return clipkg.NewCommandBuilder("get <id>").
		WithShort("Get an object by ID").
		WithHelpBuilder(
			clipkg.DynamicHelpBuilder(
				"Get an object by ID",
				"Get an object by its ID.",
			).
				AddExample("Get a backlog item", "%s get ITEM-626").
				ExcludeCommonFlags(),
		).
		WithArgs(cobra.ExactArgs(1)).
		WithRunE(runGet).
		WithCommonFlags(true).
		Build()
}
```

Example 2: CRUD Command with Standard Flags

Before:
```go
func NewDeleteCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(...)
	deleteCmd := &cobra.Command{
		Use:  "delete <id> [flags]",
		Args: cobra.ExactArgs(1),
		RunE: runDelete,
	}
	helpBuilder.ApplyToCommand(deleteCmd)
	cli.AddCommonFlags(deleteCmd)
	deleteCmd.Flags().Bool("cascade", false, "Delete object and all objects that reference it")
	deleteCmd.Flags().Bool("dry-run", false, "Show what would be deleted")
	return deleteCmd
}
```

After (using CRUDCommandBuilder):
```go
func NewDeleteCmd() *cobra.Command {
	return clipkg.NewCRUDCommandBuilder("delete", "delete <id> [flags]").
		WithCRUDHelp(
			"Delete an object by ID",
			"Delete an object by its ID.",
			"Delete an object", "%s delete ITEM-626",
			"Delete with cascade", "%s delete ITEM-626 --cascade",
		).
		WithArgs(cobra.ExactArgs(1)).
		WithRunE(runDelete).
		WithCascadeFlag().
		WithDryRunFlag().
		Build()
}
```

Example 3: Command with Multiple Custom Flags

Before:
```go
func NewCreateCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(...)
	createCmd := &cobra.Command{
		Use:  "create <kind> [flags]",
		Args: cobra.ExactArgs(1),
		RunE: runCreate,
	}
	helpBuilder.ApplyToCommand(createCmd)
	cli.AddCommonFlags(createCmd)
	createCmd.Flags().String("file", "", "Path to YAML file")
	createCmd.Flags().String("data", "", "Inline YAML data")
	createCmd.Flags().Bool("dry-run", false, "Show what would be created")
	createCmd.Flags().Bool("keep-file", false, "Keep the source file")
	createCmd.Flags().Bool("relaxed", false, "Relax integrity constraints")
	createCmd.Flags().Bool("force", false, "Force overwrite existing object")
	return createCmd
}
```

After (using CRUDCommandBuilder):
```go
func NewCreateCmd() *cobra.Command {
	return clipkg.NewCRUDCommandBuilder("create", "create <kind> [flags]").
		WithCRUDHelp(
			"Create a new object",
			"Create a new object of the specified kind.",
			"Create from file", "%s create backlog_item --file item.yaml",
			"Create from inline data", "%s create backlog_item --data 'title: \"New Item\"'",
		).
		WithArgs(cobra.ExactArgs(1)).
		WithRunE(runCreate).
		WithDataInputFlags().
		WithDryRunFlag().
		AddBoolFlag("keep-file", "", false, "Keep the source file after creation").
		AddBoolFlag("relaxed", "", false, "Relax integrity constraints during creation").
		AddBoolFlag("force", "", false, "Force overwrite existing object").
		Build()
}
```

Benefits:
1. Less boilerplate - common patterns are abstracted
2. Consistency - all commands follow same structure
3. Type safety - builder methods ensure correct flag types
4. Maintainability - changes to common patterns happen in one place
5. Discoverability - builder methods make available options clear
*/
