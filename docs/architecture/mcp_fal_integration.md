# Architecture Blueprint: ZQK MCP Fal.ai Integration

## 1. Overview
The `zqk-mcp-fal` server is a specialized Model Context Protocol (MCP) implementation designed to integrate the swarm with [Fal.ai](https://fal.ai/). Its primary purpose is to expose the `generate_video` tool, resolving the critical bottleneck in AV Video Commercial generation by allowing AI agents to directly synthesize and retrieve generated videos.

## 2. High-Level Architecture
The server will be implemented as a standalone binary in the ZQK ecosystem, adhering to our standard CLI-first and MCP architectural patterns.

- **Entrypoint**: `cmd/utilities/zqk-mcp-fal/main.go`
- **Core Package**: Relies on `pkg/mcp` for the underlying MCP server framing and JSON-RPC handling.
- **Service Integration**: Communicates with Fal.ai REST endpoints or WebSockets for submitting video generation requests and polling/receiving results.

## 3. Tool Specification: `generate_video`
The MCP server will expose a single, powerful tool: `generate_video`, capable of both Text-to-Video and Image-to-Video generation.

### Inputs (JSON Schema)
*   **`prompt`** (string, required): The detailed description of the video to generate.
*   **`model`** (string, optional): The specific Fal.ai model to use. 
    - Text-to-Video: `fal-ai/kling-video/v1/standard/text-to-video`
    - Image-to-Video (Character Animation): `fal-ai/kling-video/o3/standard/image-to-video`
*   **`image_url`** (string, optional): Required ONLY for Image-to-Video. The starting frame image URL to bring static characters/subjects to life.
*   **`end_image_url`** (string, optional): Supported on O3 models. Defines the final frame for strict transition control and continuity between narrative sequences.
*   **`aspect_ratio`** (string, optional): E.g., `16:9`, `9:16`, `1:1`.
*   **`duration`** (string, optional): Desired length of the video (e.g., `'5'` or `'10'`).

### Outputs
*   **`video_url`**: The URL to the generated `.mp4` file.
*   **`status`**: Success/failure.
*   **`metadata`**: Generation timing, cost, and specific model details.

### Narrative Continuity (Image-to-Video)
To satisfy the requirement of character-driven narratives, the swarm must utilize the Image-to-Video endpoints. By feeding highly detailed character images (`image_url`) and utilizing deterministic end frames (`end_image_url`), agents can stitch together continuous 5-10 second sequences that maintain spatial and temporal cohesion across a unified narrative.

## 4. Component Breakdown

### 4.1. `cmd/utilities/zqk-mcp-fal/main.go`
The main executable. Responsibilities:
- Parse CLI flags (e.g., `--port`, `--fal-key`).
- Read configuration/environment variables (specifically `FAL_KEY`).
- Initialize the MCP server using `pkg/mcp`.
- Register the `generate_video` tool and its corresponding handler.
- Start the stdio or HTTP/SSE listener for the MCP protocol.

### 4.2. `pkg/mcp/fal/client.go` (or `internal/fal/client.go`)
A lightweight client wrapper for the Fal.ai API.
- Handles authentication via `Authorization: Key <FAL_KEY>`.
- Submits jobs to Fal.ai.
- Polls for completion status (since video generation is asynchronous) or uses webhooks if applicable.
- Handles errors (e.g., rate limits, invalid prompts).

### 4.3. Tool Handler (`server.go`)
Maps the MCP tool invocation for `generate_video` to the Fal client.
- Validates the incoming parameters from the agent.
- Initiates the Fal.ai generation job.
- Awaits completion (with appropriate timeout handling via contexts).
- Returns the final video URL and metadata in the MCP result format.

## 5. Security & Configuration
- **API Keys**: `FAL_KEY` must be provided via environment variables. The server should fail to start if this is missing.
- **Traceability**: All generations should be logged using ZQK's structured logging framework (`logging.Fluent`). Any direct `fmt.Print` calls are strictly forbidden per `POL-CODE-007`. 
- **Timeouts**: Due to the long-running nature of video generation, the handler must implement robust context timeouts and keep-alives to prevent the MCP connection from dropping.

## 6. Implementation Steps
1. Create the scaffolding in `cmd/utilities/zqk-mcp-fal/`.
2. Implement the Fal.ai API client to trigger video generation and poll for results.
3. Wire the tool `generate_video` into the MCP server instance.
4. Add unit tests for the parameter validation and Fal client mocking.
5. Update `Makefile` to include `make bin/zqk-mcp-fal`.
