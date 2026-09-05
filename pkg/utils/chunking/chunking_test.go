package chunking

import (
	"bytes"
	"crypto/rand"
	mrand "math/rand"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

func TestChunkerAndReassembler(t *testing.T) {
	tests := []struct {
		name      string
		dataSize  int
		chunkSize int
	}{
		{"Empty data", 0, 1024},
		{"Single chunk exact size", 1024, 1024},
		{"Single chunk small", 500, 1024},
		{"Multiple chunks exact", 2048, 1024},
		{"Multiple chunks unaligned", 2500, 1024},
		{"Large payload", 1024 * 1024, 65536},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalData := make([]byte, tt.dataSize)
			if tt.dataSize > 0 {
				_, err := rand.Read(originalData)
				if err != nil {
					t.Fatalf("failed to generate random data: %v", err)
				}
			}

			reader := bytes.NewReader(originalData)
			chunkChan := make(chan Chunk)

			var wg sync.WaitGroup
			wg.Add(1)
			goroutinelabels.NewGoroutine("test", "test").StartSimple(func() {
				defer wg.Done()
				err := Chunker(reader, tt.chunkSize, chunkChan)
				if err != nil {
					t.Errorf("chunker failed: %v", err)
				}
			})

			var chunks []Chunk
			for chunk := range chunkChan {
				chunks = append(chunks, chunk)
			}
			waitDone := make(chan struct{})
			goroutinelabels.NewGoroutine("test", "test").StartSimple(func() {
				wg.Wait()
				close(waitDone)
			})
			select {
			case <-waitDone:
			case <-time.After(30 * time.Second):
				t.Fatalf("timeout waiting for wg")
			}

			// Validate last chunk flag
			if len(chunks) > 0 {
				if !chunks[len(chunks)-1].IsLast {
					t.Errorf("expected last chunk to have IsLast=true")
				}
				for i := 0; i < len(chunks)-1; i++ {
					if chunks[i].IsLast {
						t.Errorf("chunk %d prematurely marked as last", i)
					}
				}
			}

			// Reassemble in order
			var output bytes.Buffer
			reassembler := NewReassembler(&output)

			for _, c := range chunks {
				err := reassembler.AddChunk(c)
				if err != nil {
					t.Fatalf("failed to add chunk: %v", err)
				}
			}

			if !reassembler.IsComplete() {
				t.Errorf("reassembler should be complete")
			}

			if !bytes.Equal(originalData, output.Bytes()) {
				t.Errorf("reassembled data does not match original")
			}
		})
	}
}

func TestReassemblerOutOfOrder(t *testing.T) {
	originalData := make([]byte, 5000)
	rand.Read(originalData)

	reader := bytes.NewReader(originalData)
	chunkChan := make(chan Chunk)

	var wg sync.WaitGroup
	wg.Add(1)
	goroutinelabels.NewGoroutine("test", "test").StartSimple(func() {
		defer wg.Done()
		err := Chunker(reader, 1024, chunkChan)
		if err != nil {
			t.Errorf("chunker failed: %v", err)
		}
	})

	var chunks []Chunk
	for chunk := range chunkChan {
		chunks = append(chunks, chunk)
	}
	wg.Wait()

	// Shuffle chunks
	r := mrand.New(mrand.NewSource(time.Now().UnixNano()))
	r.Shuffle(len(chunks), func(i, j int) {
		chunks[i], chunks[j] = chunks[j], chunks[i]
	})

	var output bytes.Buffer
	reassembler := NewReassembler(&output)

	for _, c := range chunks {
		err := reassembler.AddChunk(c)
		if err != nil {
			t.Fatalf("failed to add chunk: %v", err)
		}
	}

	if !reassembler.IsComplete() {
		t.Errorf("reassembler should be complete")
	}

	if !bytes.Equal(originalData, output.Bytes()) {
		t.Errorf("reassembled data does not match original")
	}
}

func TestReassemblerHashTampering(t *testing.T) {
	originalData := []byte("hello world data for tampering test")
	reader := strings.NewReader(string(originalData))
	chunkChan := make(chan Chunk)

	var wg sync.WaitGroup
	wg.Add(1)
	goroutinelabels.NewGoroutine("test", "test").StartSimple(func() {
		defer wg.Done()
		err := Chunker(reader, 5, chunkChan)
		if err != nil {
			t.Errorf("chunker failed: %v", err)
		}
	})

	var chunks []Chunk
	for chunk := range chunkChan {
		chunks = append(chunks, chunk)
	}
	wg.Wait()

	// Tamper with the data of the second chunk
	chunks[1].Data[0] ^= 0xFF

	var output bytes.Buffer
	reassembler := NewReassembler(&output)

	var err error
	for _, c := range chunks {
		err = reassembler.AddChunk(c)
		if err != nil {
			break
		}
	}

	if err == nil {
		t.Errorf("expected hash verification failure, but got nil")
	} else if !strings.Contains(err.Error(), "hash verification failed") {
		t.Errorf("expected hash verification failed error, got %v", err)
	}
}

func TestChunkerInvalidChunkSize(t *testing.T) {
	reader := strings.NewReader("some data")
	chunkChan := make(chan Chunk)
	err := Chunker(reader, 0, chunkChan)
	if err == nil {
		t.Errorf("expected error for chunk size <= 0")
	}
}
