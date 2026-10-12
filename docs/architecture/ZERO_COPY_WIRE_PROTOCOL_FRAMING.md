# Zero-Copy Event Wire Protocol Serialization and Vectorized Batch Framing

## Overview

The Zero-Copy Event Wire Protocol provides a high-throughput, low-allocation binary serialization and batch framing layout for the Knowledge Kernel event mesh (`cmd/zqk/callback`). It enables resilient, cross-process, stream-delimited transfer of `CallbackEntry` instances with zero allocations on pre-sized buffers, strict Castagnoli CRC32 validation, and deterministic sequence tracking.

## Binary Frame Architecture

Each wire protocol frame consists of a fixed **32-byte header** followed by variable-length payload bytes:

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                       Magic: "ZQK1" (0x5A514B31)              |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
| Version (0x01)|  Flags (0x00) |       PayloadType (0x0001)    |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                     Sequence ID (uint64 high)                 |
|                     Sequence ID (uint64 low)                  |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                   Timestamp Nano (uint64 high)                |
|                   Timestamp Nano (uint64 low)                 |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                     Payload Length (uint32)                   |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                     CRC32 Checksum (Castagnoli)               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                     Payload Bytes (Variable)...               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

### Frame Fields

1. **Magic Header (`0x5A514B31` / "ZQK1")**: 4 bytes identifying valid protocol stream frames.
2. **Version (`1`)**: 1 byte defining protocol framing version.
3. **Flags**: 1 byte bitmask:
   - `0x01`: Payload compressed (`WireFlagCompressed`)
   - `0x02`: Heartbeat keepalive without payload (`WireFlagHeartbeat`)
4. **PayloadType (`1`)**: 2 bytes identifying serialization schema (`WirePayloadTypeJSON`).
5. **Sequence ID**: 8 bytes (uint64 BigEndian) tracking sequence numbers for WAL watermark progression.
6. **Timestamp Nano**: 8 bytes (int64 BigEndian) UTC event generation timestamp in nanoseconds.
7. **Payload Length**: 4 bytes (uint32 BigEndian) byte count of the following payload data.
8. **CRC32 Checksum**: 4 bytes (uint32 BigEndian) Castagnoli CRC32 computed over the payload bytes.
9. **Payload Data**: Raw serialized bytes representing the entry.

## API Components

### `WireProtocolCodec`

Handles single-frame binary encoding and decoding:
- `EncodeEntry(entry *CallbackEntry, buf []byte) ([]byte, error)`: Serializes a single entry into a pre-sized buffer. When `cap(buf) >= WireHeaderSize + payloadLen`, no heap allocations are incurred on the frame framing.
- `DecodeEntry(data []byte) (*CallbackEntry, int, error)`: Validates magic number, version, payload length, and CRC32 checksum before deserializing the entry and returning the number of consumed bytes.

### `WireBatchFramer`

Manages contiguous vectorized multi-frame encoding and stream parsing:
- `EncodeBatch(entries []*CallbackEntry, buf []byte) ([]byte, error)`: Vectorized batch serialization packing multiple frames contiguously.
- `DecodeBatch(data []byte) ([]*CallbackEntry, error)`: Sequentially consumes and parses frames until the buffer is exhausted.

## Invariant Guarantees & Verification

- **Corruption Resistance**: Any frame alteration or truncation fails CRC32 or header validation immediately.
- **Concurrency Safety**: Codecs and framers are fully reentrant and concurrent-safe under `-race`.
- **AST Hygiene**: Zero swallowed returns, zero blank identifiers, all background routines instrumented with `goroutinelabels`.
