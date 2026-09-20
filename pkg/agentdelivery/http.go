package agentdelivery

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/httpheaders"
	"github.com/zqk-os/zqk/pkg/specbuilder"
)

// httpDoer is satisfied by *http.Client and specbuilder.APIClient.
type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// HTTPDeliverer POSTs application/json with the markdown body and session metadata.
// Optional BearerToken sets Authorization: Bearer when non-empty.
// URLs must use http or https with a non-empty host; see parseHTTPDeliveryURL.
// For custom TLS, timeouts, or HTTP/2, set Client to a configured *http.Client.
// When Client is nil, the default uses the APISpec builder (telemetry + resiliency).
// TRACK: BLI-1783761336286408000-ca1625db
type HTTPDeliverer struct {
	URL         string
	BearerToken string
	Client      *http.Client
}

// Name implements Deliverer.
func (h HTTPDeliverer) Name() string { return "http" }

type httpDeliveryPayload struct {
	Format               string `json:"format"`
	ConvergenceSessionID string `json:"convergence_session_id"`
	Markdown             string `json:"markdown"`
}

// Deliver implements Deliverer.
func (h HTTPDeliverer) Deliver(ctx context.Context, p Prompt) (Result, error) {
	endpoint, err := normalizedHTTPDeliveryEndpoint(h.URL)
	if err != nil {
		return Result{}, err
	}
	body, err := marshalHTTPDeliveryPayload(p)
	if err != nil {
		return Result{}, err
	}
	req, err := newHTTPDeliveryPOSTRequest(ctx, endpoint, body, h.BearerToken)
	if err != nil {
		return Result{}, err
	}
	client := httpClientOrDefault(h.Client)
	resp, err := client.Do(req)
	if err != nil {
		return Result{}, errfmt.Newf("agentdelivery: http post").Wrap(err)
	}
	return finishHTTPDelivery(endpoint, resp)
}

func normalizedHTTPDeliveryEndpoint(raw string) (string, error) {
	u, err := parseHTTPDeliveryURL(raw)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

func parseHTTPDeliveryURL(raw string) (*url.URL, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, errfmt.Errorf("agentdelivery: HTTPDeliverer: empty URL")
	}
	u, err := url.Parse(s)
	if err != nil {
		return nil, errfmt.Newf("agentdelivery: HTTPDeliverer: invalid URL").Wrap(err)
	}
	switch u.Scheme {
	case "http", "https":
	default:
		if u.Scheme == "" {
			return nil, errfmt.Errorf("agentdelivery: HTTPDeliverer: URL missing scheme (want http or https)")
		}
		return nil, errfmt.Errorf("agentdelivery: HTTPDeliverer: unsupported scheme %q (want http or https)", u.Scheme)
	}
	if u.Host == "" {
		return nil, errfmt.Errorf("agentdelivery: HTTPDeliverer: URL missing host")
	}
	return u, nil
}

func marshalHTTPDeliveryPayload(p Prompt) ([]byte, error) {
	body, err := json.Marshal(httpDeliveryPayload{
		Format:               p.Format,
		ConvergenceSessionID: strings.TrimSpace(p.SessionID),
		Markdown:             string(p.Markdown),
	})
	if err != nil {
		return nil, errfmt.Newf("agentdelivery: http marshal").Wrap(err)
	}
	return body, nil
}

func newHTTPDeliveryPOSTRequest(ctx context.Context, endpoint string, body []byte, bearer string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, errfmt.Newf("agentdelivery: http request").Wrap(err)
	}
	req.Header.Set(httpheaders.ContentType, "application/json")
	if t := strings.TrimSpace(bearer); t != "" {
		req.Header.Set(httpheaders.Authorization, "Bearer "+t)
	}
	return req, nil
}

func httpClientOrDefault(c *http.Client) httpDoer {
	if c != nil {
		return c
	}
	return sharedAPISpecHTTPClient()
}

var (
	apiSpecHTTPOnce   sync.Once
	apiSpecHTTPShared httpDoer
)

func sharedAPISpecHTTPClient() httpDoer {
	apiSpecHTTPOnce.Do(func() {
		spec := specbuilder.APISpec{
			Name:    "agentdelivery-http",
			Timeout: 30 * time.Second,
			RetryPolicy: specbuilder.RetryPolicy{
				MaxRetries: 2,
				Backoff:    time.Second,
			},
		}
		// Telemetry only: resiliency retries re-POST without GetBody and break
		// bytes.Reader bodies (ContentLength with Body length 0).
		client, err := specbuilder.NewBuilder().
			WithSpec(spec).
			WithTelemetry(true).
			WithResiliency(false).
			Build()
		if err != nil || client == nil {
			client, _ = specbuilder.NewBuilder().WithSpec(spec).Build()
		}
		apiSpecHTTPShared = client
	})
	return apiSpecHTTPShared
}

func finishHTTPDelivery(endpoint string, resp *http.Response) (Result, error) {
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	tag := "http:" + endpoint
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return Result{DeliveredTo: tag, HTTPStatusCode: resp.StatusCode},
			errfmt.Errorf("agentdelivery: http status %d", resp.StatusCode)
	}
	return Result{DeliveredTo: tag, HTTPStatusCode: resp.StatusCode}, nil
}
