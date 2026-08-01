# Visual Plan Object Schema

Below is the proposed schema for the new `visual_plan` object. By rigidly enforcing this data contract, we remove creative interpretation from the video model, empower marketing to define the strategic vision, and give production a foolproof blueprint for continuity and quality.

No API requests will be fired until this schema is populated and explicitly approved.

## Object Definition: `visual_plan`

```yaml
id: VIS-####
kind: visual_plan
schema_version: "1.1.0"

# Marketing's Strategic Vision
# Dictates the exact social media formula and impact requirements
marketing_vision:
  title: "ZQK Enterprise Commercial - Phase 3"
  target_audience: "CIOs, CTOs, Enterprise Architects"
  core_aesthetic: "Prism/Flow, clean, minimalist, high-end tech"
  global_tone: "Calm, Authoritative, Visionary"
  story_framework: "AIDA (Attention, Interest, Desire, Action)" # Standard industry formula
  narrative_arc:
    hook: "The overwhelming chaos of ungoverned AI agents."
    build: "The introduction of the Knowledge Kernel as a clarifying prism."
    climax: "Total, effortless synchronization across a global enterprise."
    cta: "Stop running naked agents. Embrace ZQK."

# Production's Global Constraints
# Eliminates guesswork for continuity and brand alignment
project_context:
  brand_asset_refs:
    - "AST-1001" # Link to official ZQK logo SVG
    - "AST-1002" # Link to official typography/font package
    - "AST-1005" # Link to precise color palette definitions
  
  characters:
    - character_id: "CHAR-01"
      role: "Lead Engineer"
      reference_image_url: "https://zqk.internal/assets/chars/lead_engineer_ref.png"
      attire_consistency: "Sharp tailored navy blue suit, white shirt, no tie. Silver minimalist watch on left wrist. Attire must not change or morph across shots."
      facial_consistency: "Short cropped hair, glasses with thin black frames."

# The structured timeline
scenes:
  - scene_id: "SCN-001"
    narrative_phase: "hook" # Links back to the marketing vision
    description: "Introduction to the concept of chaotic data being crystallized into structure."
    
    # Scene-wide visual constraints
    color_palette: 
      - "#1A202C" # Dark Blue (Background)
      - "#00FFFF" # Light Blue (Accent)
      - "#FFFFFF" # Crisp White (Highlights)
    
    # Micro-interval definitions
    shots:
      - shot_id: "SHT-001A"
        duration_ms: 1000
        
        # State memory for precise chaining between shots
        memory:
          previous_shot_id: null
          anchor_image_ref: null
        
        # Exact image generation constraints
        composition:
          character_refs: ["CHAR-01"] # Explicitly invokes the character consistency rules above
          subject: "CHAR-01 standing amidst abstract scattered light particles floating in dark space."
          camera_angle: "Medium close-up, eye level"
          lighting: "High-contrast architectural lighting, crisp shadows"
          style: "Fluid dynamics simulation, ultra-realistic, 8k, Octane Render"
        
        # Exact video generation constraints
        animation:
          movement_amplitude: 1       # 1-10 scale (1 = almost frozen, prevents hallucination)
          camera_motion: "subtle, slow pan right"
          locked: true                # Enforces zero character/subject morphing
        
        # Audio & Transition
        voiceover_text: "The era of untethered AI is chaotic."
        transition_out: "crossfade_500ms"
        model_target: "fal-ai/kling-3"
        
      - shot_id: "SHT-001B"
        duration_ms: 1000
        
        memory:
          previous_shot_id: "SHT-001A"
          anchor_image_ref: "extract_last_frame(SHT-001A)"
          
        composition:
          character_refs: ["CHAR-01"]
          subject: "The light particles begin to slow down and align along an invisible grid around CHAR-01."
          camera_angle: "Medium close-up, tracking"
          lighting: "High-contrast architectural lighting, crisp shadows"
          style: "Fluid dynamics simulation, ultra-realistic, 8k, Octane Render"
        
        animation:
          movement_amplitude: 2
          camera_motion: "slow push in"
          locked: true
        
        voiceover_text: "Unpredictable."
        transition_out: "hard_cut"
        model_target: "fal-ai/kling-3"
```

### Key Mechanisms:
1. **`marketing_vision`**: Captures the proven social media formulas (like AIDA) and narrative arcs, ensuring the marketing team defines the impact and storyline precisely.
2. **`characters` & `brand_asset_refs`**: Eliminates production guesswork. By explicitly defining `attire_consistency`, `facial_consistency`, and linking to exact `brand_asset_refs` (logos, fonts, palettes), the AI models are forced into strict continuity bounds.
3. **`character_refs` inside Shots**: Rather than rewriting character descriptions, the composition simply references `"CHAR-01"`. The production script will automatically inject the locked attire and facial constraints into the image generation prompt to guarantee frame-to-frame continuity.
