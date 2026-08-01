# Digital Asset Object Schema

To support the Symbiotic Mesh architecture, ZQK must remain lightweight and distributed. Large binary files (videos, images, audio) are never stored inside the Knowledge Kernel. Instead, the kernel stores `digital_asset` objects—lightweight metadata pointers that rank the asset's quality and define its physical location (whether on a local laptop, an S3 bucket, or a decentralized CMS). 

By strictly decoupling the knowledge graph from the binary blob storage, we ensure that individual ZQK nodes remain fast, easily snapshot-able, and can be distributed across different domains or corporate divisions.

## Object Definition: `digital_asset`

```yaml
id: AST-####
kind: digital_asset
schema_version: "1.0.0"

# Metadata describing what the asset is
metadata:
  title: "Scene 1 - Final Cut (Prism Reveal)"
  asset_type: "video/mp4"
  mime_type: "video/mp4"
  file_size_bytes: 8466811
  resolution: "1920x1080"
  duration_ms: 5000

# Decentralized Storage & Mesh Routing
storage:
  # The URI specifies the protocol and location. 
  # This can evolve from 'local://' to 's3://', 'ipfs://', or a custom 'cms://' 
  primary_uri: "local:///Users/lanceettl/ai-projects/zqk/docs/marketing/assets/final_commercial.mp4"
  
  # Which ZQK kernel instance/domain is responsible for stewarding this asset
  steward_node: "zqk-marketing-division-primary"
  
  # Backup URIs for fault tolerance across the mesh
  replica_uris: []

# Quality Control & Feedback Loop
quality_metrics:
  # 0-100 score utilized by the CAP loop to rank outputs
  composite_score: 92
  
  # Specific grading criteria
  character_continuity_score: 95
  prompt_adherence_score: 90
  visual_fidelity_score: 98
  
  # Has a human (or senior agent) approved this for external release?
  status: "approved" # enum: generated, reviewing, rejected, approved

# Provenance (Traceability back to the generation source)
lineage:
  generated_by: "fal-ai/kling-3"
  source_plan_ref: "VIS-0012" # Links back to the visual_plan that requested it
  source_shot_ref: "SHT-001B" # Links back to the exact micro-interval definition
```

### Mesh Architecture Benefits:
1. **Lightweight Nodes**: The ZQK graph only stores the YAML representation above. A node can hold millions of `digital_asset` references without suffering performance degradation, allowing for rapid backups and state restores.
2. **Domain Stewarding**: The `steward_node` field allows a corporate ecosystem to split the swarm. The "Marketing Node" stewards commercial videos, while the "Engineering Node" stewards architectural diagrams. They share knowledge (the YAML objects) seamlessly across the mesh without transferring gigabytes of binary data unless explicitly requested.
3. **Quality Ranking**: The `quality_metrics` block allows the swarm to automatically generate 5 variations of a video clip, rank them based on continuity and prompt adherence, and automatically select the highest `composite_score` for the final stitch.
