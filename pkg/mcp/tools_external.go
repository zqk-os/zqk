package mcp

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage/signedurl"
	"github.com/lanceman/zqk/pkg/utils/chunking"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// RegisterExternalTools registers tools for handling external data securely
func RegisterExternalTools(server *Server) {
	// get_signed_url tool
	server.RegisterTool(
		"get_signed_url",
		"Generates a cryptographically signed URL for secure out-of-band data transfer. Use this to transfer massive blobs or interact with external data without saturating the MCP Control-Plane.",
		map[string]any{
			objects.FieldKeyType: "object",
			"required":           []string{"path", "ttl_seconds"},
			"properties": map[string]any{
				objects.FieldKeyPath: map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "The file or resource path to sign",
				},
				"ttl_seconds": map[string]any{
					objects.FieldKeyType:        "number",
					objects.FieldKeyDescription: "Time-to-live for the signed URL in seconds",
				},
			},
		},
		HandleGetSignedUrl,
	)

	// fetch_chunk tool
	server.RegisterTool(
		"fetch_chunk",
		"Fetches a specific chunk of a large file to avoid single-blob JSON-RPC transfers.",
		map[string]any{
			objects.FieldKeyType: "object",
			"required":           []string{"path", "chunk_size", "chunk_index"},
			"properties": map[string]any{
				objects.FieldKeyPath: map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "The file path to fetch",
				},
				"chunk_size": map[string]any{
					objects.FieldKeyType:        "number",
					objects.FieldKeyDescription: "Size of each chunk in bytes",
				},
				"chunk_index": map[string]any{
					objects.FieldKeyType:        "number",
					objects.FieldKeyDescription: "The 0-based index of the chunk to fetch",
				},
			},
		},
		HandleFetchChunk,
	)
}

// HandleGetSignedUrl handles the generation of signed URLs
func HandleGetSignedUrl(ctx context.Context, args map[string]any) (any, error) {
	path, ok := args[objects.FieldKeyPath].(string)
	if !ok {
		return nil, fmt.Errorf("path is required")
	}

	ttlRaw, ok := args["ttl_seconds"].(float64)
	if !ok {
		return nil, fmt.Errorf("ttl_seconds is required")
	}

	secret := os.Getenv(zqkenv.MCPExternalSecretKey())
	if secret == "" {
		return nil, fmt.Errorf("ZQK_MCP_EXTERNAL_SECRET_KEY environment variable is not set")
	}
	signer := signedurl.NewSigner(secret)

	ttl := time.Duration(ttlRaw) * time.Second
	url, err := signer.Generate(path, ttl)
	if err != nil {
		return nil, fmt.Errorf("failed to generate signed url: %w", err)
	}

	return map[string]any{
		"signed_url": url,
		"expires_in": ttlRaw,
	}, nil
}

// HandleFetchChunk handles fetching a specific chunk of a large file
func HandleFetchChunk(ctx context.Context, args map[string]any) (any, error) {
	path, ok := args[objects.FieldKeyPath].(string)
	if !ok {
		return nil, fmt.Errorf("path is required")
	}

	chunkSizeRaw, ok := args["chunk_size"].(float64)
	if !ok || chunkSizeRaw <= 0 {
		return nil, fmt.Errorf("invalid chunk_size")
	}
	chunkSize := int(chunkSizeRaw)

	chunkIndexRaw, ok := args["chunk_index"].(float64)
	if !ok || chunkIndexRaw < 0 {
		return nil, fmt.Errorf("invalid chunk_index")
	}
	chunkIndex := int(chunkIndexRaw)

	file, err := fileutil.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Seek to the correct chunk
	offset := int64(chunkSize * chunkIndex)
	_, err = file.Seek(offset, io.SeekStart)
	if err != nil {
		return nil, fmt.Errorf("failed to seek: %w", err)
	}

	// Read just this chunk using LimitReader
	limitReader := io.LimitReader(file, int64(chunkSize))

	out := make(chan chunking.Chunk)
	errChan := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	goroutinelabels.NewGoroutine("refactored_worker", "Refactored raw goroutine").
		StartSimple(func() {
			func() {
				defer wg.Done()
				errChan <- chunking.Chunker(limitReader, chunkSize, out)
			}()
		})

	var chunks []chunking.Chunk
	for c := range out {
		chunks = append(chunks, c)
	}

	waitDone := make(chan struct{})
	goroutinelabels.NewGoroutine("wait_chunker", "Wait for chunking").
		StartSimple(func() {
			wg.Wait()
			close(waitDone)
		})
	select {
	case <-waitDone:
	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("timeout waiting for chunker")
	}

	if chunkErr := <-errChan; chunkErr != nil {
		return nil, fmt.Errorf("chunking failed: %w", chunkErr)
	}

	if len(chunks) == 0 {
		return nil, fmt.Errorf("chunk not found")
	}

	c := chunks[0]
	// Determine if it's the last chunk
	fileInfo, err := file.Stat()
	isLast := false
	if err == nil {
		if offset+int64(len(c.Data)) >= fileInfo.Size() {
			isLast = true
		}
	} else {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	return map[string]any{
		"sequence_id": chunkIndex,
		"is_last":     isLast,
		"hash":        c.Hash,
		"data":        c.Data,
	}, nil
}
