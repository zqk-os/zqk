package io

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clictx "github.com/zqk-os/zqk/pkg/cliapp/context"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	iopkg "github.com/zqk-os/zqk/pkg/io"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func executeIO(t *testing.T, projectRoot string, args ...string) (string, error) {
	t.Helper()
	cmd := NewIOCmd()
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd.SetContext(ctx)
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cliCtx := &cli.Context{
		Context: &clictx.Context{
			ProjectRoot: projectRoot,
			Profile:     "debug",
		},
	}
	cli.SetContext(cmd, cliCtx)
	for _, sub := range cmd.Commands() {
		cli.SetContext(sub, cliCtx)
	}
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestNewIOCmd_Structure(t *testing.T) {
	cmd := NewIOCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "io", cmd.Use)

	var hasFetch bool
	for _, sub := range cmd.Commands() {
		if sub.Name() == "fetch" {
			hasFetch = true
			break
		}
	}
	assert.True(t, hasFetch, "io command must have fetch subcommand")
}

func TestIO_Fetch_Lifecycle(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)

	serverPayload := `<!DOCTYPE html>
<html>
<head><script>alert("tracker");</script></head>
<body>
<h1>Respiration Gateway</h1>
<p>Universal external content ingester for ZQK.</p>
<footer>Footer discard</footer>
</body>
</html>`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(serverPayload))
	}))
	defer ts.Close()

	// 1. Fetch plain text
	out, err := executeIO(t, proj.Root, "fetch", ts.URL)
	require.NoError(t, err, "fetch failed: %s", out)
	assert.Contains(t, out, "# Respiration Gateway")
	assert.Contains(t, out, "Universal external content ingester for ZQK.")
	assert.NotContains(t, out, "tracker")
	assert.NotContains(t, out, "Footer discard")
	assert.Contains(t, out, "Clean Hash:")

	// 2. Fetch JSON format with --cache-cas
	jsonOut, err := executeIO(t, proj.Root, "fetch", ts.URL, "--cache-cas", "--json")
	require.NoError(t, err, "fetch --cache-cas --json failed: %s", jsonOut)

	var res iopkg.FetchResult
	err = json.Unmarshal([]byte(jsonOut), &res)
	require.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	assert.True(t, res.Cached)
	assert.Contains(t, res.CleanContent, "# Respiration Gateway")

	// 3. Mutate server response and fetch with --diff
	serverPayload = `<!DOCTYPE html>
<html>
<body>
<h1>Respiration Gateway</h1>
<p>Universal external content ingester for ZQK with real-time CAS scraping.</p>
</body>
</html>`

	diffOut, err := executeIO(t, proj.Root, "fetch", ts.URL, "--diff")
	require.NoError(t, err, "fetch --diff failed: %s", diffOut)
	assert.Contains(t, diffOut, "SEMANTIC DIFF:")
	assert.Contains(t, diffOut, "- Universal external content ingester for ZQK.")
	assert.Contains(t, diffOut, "+ Universal external content ingester for ZQK with real-time CAS scraping.")

	// 4. Fetch with --raw
	rawOut, err := executeIO(t, proj.Root, "fetch", ts.URL, "--raw")
	require.NoError(t, err)
	assert.Contains(t, rawOut, "<html>")
	assert.Contains(t, rawOut, "real-time CAS scraping")
}
