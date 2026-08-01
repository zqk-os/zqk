# Initialization Seed Questions v1.0

**Version**: 1.0.0  
**Created**: 2025-12-30  
**Status**: Design  
**Related**: Project Discovery v1.0, Role Discovery v1.0, Authority Resolution v1.0

## Overview

This document defines a comprehensive seed question system that must be answered before zqk can be considered "initialized". The system generates seed question files that prompt users to provide answers, which help zqk determine the environment in which it operates. It also handles edge cases like partnerships where one organization owns code but a partner builds features.

## Problem Statement

**Challenges**:

1. **Initialization Requirements**: System needs seed questions answered before it can operate
2. **Environment Detection**: Answers help determine operating environment
3. **Partnership Scenarios**: One org owns code, partner builds features, different organizational alignment
4. **Question Generation**: Need to generate seed question files automatically
5. **Answer Interpretation**: Answers must be interpreted to configure system appropriately

**Example Scenarios**:

- **Standard Project**: Single organization, clear ownership
- **Partnership**: Organization A owns code, Organization B builds features
- **Multi-Organization**: Multiple organizations with different strategic alignment
- **Rapid Startup**: Fast-moving startup needs quick initialization

## Solution: Seed Question System

### Core Principles

1. **Seed Questions First**: Must answer seed questions before system is initialized
2. **Environment Detection**: Answers determine operating environment
3. **Partnership Support**: Handles complex organizational relationships
4. **Progressive Disclosure**: Start with essential questions, add context-specific ones
5. **Answer Interpretation**: System interprets answers to configure appropriately
6. **Initialization Completion**: System only "initialized" when all seed questions answered

## Architecture

### 1. Seed Question Generation

**Command**: `zqk system init --seed-questions`

Generates seed question files based on context:

**Generation Process**:

#### Step 1: Context Detection
```bash
# Detect context and generate appropriate seed questions
zqk system init --seed-questions

# Output:
# Context Detected: new_project
# Generating seed questions...
# Seed questions generated in .zqk/seed/questions/
```

**Seed Question Categories**:

1. **Essential Questions** (Always Required):
   - Project type and ownership
   - Organizational structure
   - Primary stakeholders
   - Initial goals

2. **Context-Specific Questions** (Conditional):
   - Partnership questions (if partnership detected)
   - Enterprise questions (if enterprise detected)
   - Multi-organization questions (if multiple orgs detected)

3. **Advanced Questions** (Optional):
   - Strategic alignment details
   - Policy preferences
   - Integration requirements

#### Step 2: Seed Question File Structure
```
.zqk/seed/
├── questions/
│   ├── essential.yaml          # Essential questions (required)
│   ├── partnership.yaml        # Partnership questions (if applicable)
│   ├── enterprise.yaml         # Enterprise questions (if applicable)
│   └── advanced.yaml            # Advanced questions (optional)
├── answers/
│   └── answers.yaml            # User answers (filled during init)
└── interpretation/
    └── environment.yaml        # Interpreted environment configuration
```

**Essential Seed Questions**:
```yaml
# .zqk/seed/questions/essential.yaml
questions:
  - id: SEED-001
    kind: question
    question_text: "What is the project type?"
    category: essential
    required: true
    question_type: single_choice
    options:
      - "Individual project (single developer)"
      - "Team project (small team, 2-10 people)"
      - "Startup project (growing team, 10-50 people)"
      - "Enterprise project (large organization, 50+ people)"
      - "Partnership project (multiple organizations)"
    interpretation:
      individual: { customer_type: individual, complexity: minimal }
      team: { customer_type: small_team, complexity: standard }
      startup: { customer_type: startup, complexity: standard }
      enterprise: { customer_type: enterprise, complexity: enterprise }
      partnership: { customer_type: partnership, complexity: enterprise }
  
  - id: SEED-002
    kind: question
    question_text: "Who owns the code repository?"
    category: essential
    required: true
    question_type: text
    interpretation:
      field: code_owner
      usage: "Determines primary authority and ownership"
  
  - id: SEED-003
    kind: question
    question_text: "Who are the primary stakeholders?"
    category: essential
    required: true
    question_type: multi_text
    interpretation:
      field: primary_stakeholders
      usage: "Creates stakeholder profiles"
  
  - id: SEED-004
    kind: question
    question_text: "What are the initial project goals?"
    category: essential
    required: true
    question_type: multi_text
    interpretation:
      field: initial_goals
      usage: "Creates initial goal objects"
  
  - id: SEED-005
    kind: question
    question_text: "What is the project's primary domain or industry?"
    category: essential
    required: true
    question_type: text
    interpretation:
      field: domain
      usage: "Configures domain-specific policies and templates"
```

**Partnership Seed Questions**:
```yaml
# .zqk/seed/questions/partnership.yaml
questions:
  - id: SEED-PART-001
    kind: question
    question_text: "What is the partnership structure?"
    category: partnership
    required: true
    question_type: single_choice
    options:
      - "Code owner + Feature builder (one org owns code, partner builds features)"
      - "Joint development (both orgs contribute code)"
      - "Service provider (one org provides service, partner consumes)"
      - "Strategic alliance (aligned goals, separate execution)"
    interpretation:
      code_owner_feature_builder:
        structure: code_owner_feature_builder
        authority_model: hierarchical
        alignment_model: partnership_constraints
      joint_development:
        structure: joint_development
        authority_model: collaborative
        alignment_model: shared_goals
      service_provider:
        structure: service_provider
        authority_model: provider_controlled
        alignment_model: service_level
      strategic_alliance:
        structure: strategic_alliance
        authority_model: independent
        alignment_model: strategic_overlay
  
  - id: SEED-PART-002
    kind: question
    question_text: "Which organization owns the code repository?"
    category: partnership
    required: true
    question_type: text
    interpretation:
      field: code_owner_org
      usage: "Determines primary authority for code repository"
  
  - id: SEED-PART-003
    kind: question
    question_text: "Which organization builds features?"
    category: partnership
    required: true
    question_type: text
    interpretation:
      field: feature_builder_org
      usage: "Determines feature development authority"
  
  - id: SEED-PART-004
    kind: question
    question_text: "How do organizational strategic alignments differ?"
    category: partnership
    required: true
    question_type: text
    interpretation:
      field: org_alignment_differences
      usage: "Configures partnership alignment model"
  
  - id: SEED-PART-005
    kind: question
    question_text: "What are the partnership constraints that create alignment?"
    category: partnership
    required: true
    question_type: multi_text
    interpretation:
      field: partnership_constraints
      usage: "Defines alignment boundaries for partnership"
  
  - id: SEED-PART-006
    kind: question
    question_text: "How are decisions made in the partnership?"
    category: partnership
    required: true
    question_type: single_choice
    options:
      - "Code owner makes final decisions"
      - "Feature builder makes feature decisions, code owner approves"
      - "Joint decision-making (consensus required)"
      - "Separate domains (each org decides in their domain)"
    interpretation:
      field: decision_model
      usage: "Configures authority resolution and conflict resolution"
```

### 2. Interactive Question Prompting

**Command**: `zqk system init --interactive`

Prompts user to answer seed questions interactively:

**Interactive Flow**:
```bash
# Start interactive initialization
zqk system init --interactive

# Output:
# ========================================
# zqk Initialization - Seed Questions
# ========================================
# 
# Please answer the following questions to configure zqk for your environment.
# 
# [Essential Questions]
# 
# 1. What is the project type?
#   1) Individual project (single developer)
#   2) Team project (small team, 2-10 people)
#   3) Startup project (growing team, 10-50 people)
#   4) Enterprise project (large organization, 50+ people)
#   5) Partnership project (multiple organizations)
# 
# Your choice [1-5]: 5
# 
# [Partnership Questions - Detected partnership project]
# 
# 2. What is the partnership structure?
#   1) Code owner + Feature builder (one org owns code, partner builds features)
#   2) Joint development (both orgs contribute code)
#   3) Service provider (one org provides service, partner consumes)
#   4) Strategic alliance (aligned goals, separate execution)
# 
# Your choice [1-4]: 1
# 
# 3. Which organization owns the code repository?
# Your answer: Acme Corp
# 
# 4. Which organization builds features?
# Your answer: Beta Inc
# 
# 5. How do organizational strategic alignments differ?
# Your answer: |
#   Acme Corp focuses on platform stability and long-term architecture.
#   Beta Inc focuses on rapid feature delivery and customer satisfaction.
#   Different priorities but aligned on partnership success.
# 
# 6. What are the partnership constraints that create alignment?
# Your answer (one per line, empty line to finish): |
#   - Partnership agreement requires feature delivery within SLA
#   - Code quality standards must be maintained
#   - Security and compliance requirements shared
#   - Revenue sharing model creates mutual success incentive
# 
# 7. How are decisions made in the partnership?
#   1) Code owner makes final decisions
#   2) Feature builder makes feature decisions, code owner approves
#   3) Joint decision-making (consensus required)
#   4) Separate domains (each org decides in their domain)
# 
# Your choice [1-4]: 2
# 
# [Configuration Summary]
# 
# Project Type: Partnership
# Structure: Code owner + Feature builder
# Code Owner: Acme Corp
# Feature Builder: Beta Inc
# Decision Model: Feature builder makes decisions, code owner approves
# 
# Continue with initialization? (y/n): y
# 
# Initializing zqk...
# ✓ Creating directory structure
# ✓ Generating configuration files
# ✓ Creating partnership authority model
# ✓ Setting up alignment constraints
# ✓ Initializing complete
```

### 3. Answer Interpretation

**Command**: `zqk system init --interpret-answers`

Interprets answers to determine environment configuration:

**Interpretation Process**:

#### Step 1: Answer Collection
```yaml
# .zqk/seed/answers/answers.yaml
answers:
  SEED-001: "Partnership project (multiple organizations)"
  SEED-PART-001: "Code owner + Feature builder (one org owns code, partner builds features)"
  SEED-PART-002: "Acme Corp"
  SEED-PART-003: "Beta Inc"
  SEED-PART-004: |
    Acme Corp focuses on platform stability and long-term architecture.
    Beta Inc focuses on rapid feature delivery and customer satisfaction.
    Different priorities but aligned on partnership success.
  SEED-PART-005:
    - "Partnership agreement requires feature delivery within SLA"
    - "Code quality standards must be maintained"
    - "Security and compliance requirements shared"
    - "Revenue sharing model creates mutual success incentive"
  SEED-PART-006: "Feature builder makes feature decisions, code owner approves"
```

#### Step 2: Environment Interpretation
```yaml
# .zqk/seed/interpretation/environment.yaml
environment:
  project_type: partnership
  customer_type: partnership
  complexity: enterprise
  
  organizational_structure:
    type: code_owner_feature_builder
    code_owner:
      organization: "Acme Corp"
      authority_level: full_control
      responsibilities:
        - "Code repository ownership"
        - "Final approval authority"
        - "Platform stability"
        - "Long-term architecture"
    
    feature_builder:
      organization: "Beta Inc"
      authority_level: feature_development
      responsibilities:
        - "Feature development"
        - "Feature decisions (subject to approval)"
        - "Rapid delivery"
        - "Customer satisfaction"
  
  authority_model:
    type: hierarchical_with_approval
    code_owner_authority:
      - "read:*"
      - "write:*"
      - "delete:*"
      - "approve:*"
    feature_builder_authority:
      - "read:*"
      - "write:backlog_item"
      - "write:milestone"
      - "write:feature"
      - "request_approval:*"
  
  alignment_model:
    type: partnership_constraints
    alignment_boundaries:
      - "Partnership agreement SLA"
      - "Code quality standards"
      - "Security and compliance"
      - "Revenue sharing model"
    strategic_differences:
      code_owner_priorities:
        - "Platform stability"
        - "Long-term architecture"
      feature_builder_priorities:
        - "Rapid feature delivery"
        - "Customer satisfaction"
    alignment_mechanism: "Partnership constraints create alignment despite different priorities"
  
  decision_model:
    type: feature_builder_decides_owner_approves
    feature_decisions: "Beta Inc makes decisions, Acme Corp approves"
    code_decisions: "Acme Corp makes decisions"
    strategic_decisions: "Joint discussion, Acme Corp final say"
```

#### Step 3: Configuration Generation
```bash
# Generate configuration from interpreted environment
zqk system init --interpret-answers

# Output:
# Interpreting answers...
# Environment: Partnership (Code Owner + Feature Builder)
# Generating configuration...
# ✓ Created .zqk/config.yaml
# ✓ Created partnership authority model
# ✓ Created alignment constraints
# ✓ Created stakeholder profiles
# ✓ Configuration complete
```

**Generated Configuration**:
```yaml
# .zqk/config.yaml
project:
  name: "partnership-project"
  type: partnership
  customer_type: partnership
  complexity: enterprise

organizational_structure:
  type: code_owner_feature_builder
  code_owner:
    organization: "Acme Corp"
    authority_level: full_control
  feature_builder:
    organization: "Beta Inc"
    authority_level: feature_development

authority:
  model: hierarchical_with_approval
  code_owner_permissions:
    - read:*
    - write:*
    - delete:*
    - approve:*
  feature_builder_permissions:
    - read:*
    - write:backlog_item
    - write:milestone
    - write:feature
    - request_approval:*

alignment:
  model: partnership_constraints
  constraints:
    - "Partnership agreement SLA"
    - "Code quality standards"
    - "Security and compliance"
    - "Revenue sharing model"
  strategic_differences:
    code_owner: ["Platform stability", "Long-term architecture"]
    feature_builder: ["Rapid feature delivery", "Customer satisfaction"]
```

### 4. Initialization Completion Criteria

**Command**: `zqk system init --check-completion`

Checks if system is fully initialized:

**Completion Criteria**:

1. **Essential Questions Answered**: All required seed questions answered
2. **Context Questions Answered**: All context-specific questions answered (if applicable)
3. **Configuration Generated**: Environment configuration generated from answers
4. **Authority Model Created**: Authority model created based on answers
5. **Stakeholder Profiles Created**: Stakeholder profiles created from answers
6. **Initial Goals Created**: Initial goals created from answers

**Completion Check**:
```bash
# Check initialization completion
zqk system init --check-completion

# Output:
# Initialization Status:
#   ✓ Essential questions answered (5/5)
#   ✓ Partnership questions answered (6/6)
#   ✓ Configuration generated
#   ✓ Authority model created
#   ✓ Stakeholder profiles created (2/2)
#   ✓ Initial goals created (3/3)
# 
# Status: INITIALIZED
# 
# System is ready to use.
```

**Incomplete Initialization**:
```bash
# Check initialization completion
zqk system init --check-completion

# Output:
# Initialization Status:
#   ✓ Essential questions answered (5/5)
#   ✗ Partnership questions answered (4/6)
#     Missing: SEED-PART-005, SEED-PART-006
#   ✗ Configuration generated
#   ✗ Authority model created
# 
# Status: INCOMPLETE
# 
# Please complete remaining questions:
#   zqk system init --interactive
#   or
#   zqk system init --answer-file .zqk/seed/answers/answers.yaml
```

### 5. Non-Interactive Initialization

**Command**: `zqk system init --answer-file <file>`

Initialize from answer file:

**Answer File Format**:
```yaml
# answers.yaml
answers:
  SEED-001: "Partnership project (multiple organizations)"
  SEED-002: "Acme Corp"
  SEED-003:
    - "Acme Corp (code owner)"
    - "Beta Inc (feature builder)"
  SEED-004:
    - "Deliver partnership features within SLA"
    - "Maintain code quality standards"
    - "Ensure security and compliance"
  SEED-005: "SaaS Platform"
  SEED-PART-001: "Code owner + Feature builder (one org owns code, partner builds features)"
  SEED-PART-002: "Acme Corp"
  SEED-PART-003: "Beta Inc"
  SEED-PART-004: |
    Acme Corp focuses on platform stability and long-term architecture.
    Beta Inc focuses on rapid feature delivery and customer satisfaction.
  SEED-PART-005:
    - "Partnership agreement SLA"
    - "Code quality standards"
    - "Security and compliance"
    - "Revenue sharing model"
  SEED-PART-006: "Feature builder makes feature decisions, code owner approves"
```

**Non-Interactive Initialization**:
```bash
# Initialize from answer file
zqk system init --answer-file answers.yaml

# Output:
# Loading answers from answers.yaml...
# Interpreting answers...
# Environment: Partnership (Code Owner + Feature Builder)
# Generating configuration...
# ✓ Initialization complete
```

### 6. Partnership Authority Model

**Partnership Authority Configuration**:

```yaml
# Generated partnership authority model
partnership_authority:
  structure: code_owner_feature_builder
  organizations:
    - id: ORG-001
      name: "Acme Corp"
      role: code_owner
      authority:
        repositories:
          - repository: "repo:main"
            permissions: ["read:*", "write:*", "delete:*", "approve:*"]
        operations:
          - "Final approval on all changes"
          - "Code repository ownership"
          - "Platform architecture decisions"
    
    - id: ORG-002
      name: "Beta Inc"
      role: feature_builder
      authority:
        repositories:
          - repository: "repo:main"
            permissions: ["read:*", "write:backlog_item", "write:milestone", "write:feature", "request_approval:*"]
        operations:
          - "Feature development"
          - "Feature decisions (subject to approval)"
          - "Backlog item creation and management"
  
  decision_flow:
    feature_development:
      - "Beta Inc creates backlog items and features"
      - "Beta Inc develops features"
      - "Beta Inc requests approval from Acme Corp"
      - "Acme Corp reviews and approves/rejects"
      - "If approved, changes merged to main repository"
    
    code_changes:
      - "Acme Corp makes code changes directly"
      - "Beta Inc requests code changes via approval process"
  
  alignment_constraints:
    - constraint: "Partnership agreement SLA"
      applies_to: ["feature_delivery", "code_quality"]
      enforcement: "Automated checks and reporting"
    
    - constraint: "Code quality standards"
      applies_to: ["code_review", "testing", "documentation"]
      enforcement: "Pre-commit hooks and CI/CD"
    
    - constraint: "Security and compliance"
      applies_to: ["security_scanning", "compliance_checks"]
      enforcement: "Automated security scanning"
    
    - constraint: "Revenue sharing model"
      applies_to: ["feature_success_metrics", "customer_satisfaction"]
      enforcement: "Shared metrics and reporting"
```

### 7. Seed Question Templates

**Template System**:

**Template 1: Individual Developer**
```yaml
# .zqk/seed/templates/individual.yaml
essential_questions:
  - SEED-001: "Individual project"
  - SEED-002: "<user-name>"
  - SEED-003: ["<user-name>"]
  - SEED-004: ["<initial-goals>"]
  - SEED-005: "<domain>"
```

**Template 2: Partnership (Code Owner + Feature Builder)**
```yaml
# .zqk/seed/templates/partnership-code-owner.yaml
essential_questions:
  - SEED-001: "Partnership project"
partnership_questions:
  - SEED-PART-001: "Code owner + Feature builder"
  - SEED-PART-002: "<code-owner-org>"
  - SEED-PART-003: "<feature-builder-org>"
  - SEED-PART-004: "<alignment-differences>"
  - SEED-PART-005: ["<constraint-1>", "<constraint-2>"]
  - SEED-PART-006: "Feature builder makes decisions, code owner approves"
```

**Template Application**:
```bash
# Apply template
zqk system init --template partnership-code-owner \
  --code-owner "Acme Corp" \
  --feature-builder "Beta Inc"

# Output:
# Template applied
# Pre-filled answers:
#   - SEED-001: Partnership project
#   - SEED-PART-001: Code owner + Feature builder
#   - SEED-PART-002: Acme Corp
#   - SEED-PART-003: Beta Inc
# 
# Please complete remaining questions...
```

## Implementation

### Phase 1: Seed Question Generation

```go
// pkg/init/seed_questions.go
type SeedQuestionGenerator struct {
    templates map[string]*QuestionTemplate
}

func (sqg *SeedQuestionGenerator) Generate(context *InitContext) ([]*Question, error) {
    // 1. Load essential questions
    essential := sqg.LoadEssentialQuestions()
    
    // 2. Detect context and load context-specific questions
    contextQuestions := sqg.LoadContextQuestions(context)
    
    // 3. Combine questions
    allQuestions := append(essential, contextQuestions...)
    
    return allQuestions, nil
}
```

### Phase 2: Answer Interpretation

```go
// pkg/init/answer_interpreter.go
type AnswerInterpreter struct {
    interpreters map[string]AnswerInterpreterFunc
}

func (ai *AnswerInterpreter) Interpret(answers map[string]any) (*EnvironmentConfig, error) {
    // 1. Determine project type
    projectType := ai.DetermineProjectType(answers)
    
    // 2. Interpret organizational structure
    orgStructure := ai.InterpretOrganizationalStructure(answers)
    
    // 3. Generate authority model
    authorityModel := ai.GenerateAuthorityModel(orgStructure)
    
    // 4. Generate alignment model
    alignmentModel := ai.GenerateAlignmentModel(answers)
    
    return &EnvironmentConfig{
        ProjectType: projectType,
        OrganizationalStructure: orgStructure,
        AuthorityModel: authorityModel,
        AlignmentModel: alignmentModel,
    }, nil
}
```

### Phase 3: Initialization Completion

```go
// pkg/init/completion_checker.go
type CompletionChecker struct {
    requiredQuestions []string
}

func (cc *CompletionChecker) CheckCompletion(answers map[string]any) (*CompletionStatus, error) {
    // 1. Check essential questions
    essentialComplete := cc.CheckEssentialQuestions(answers)
    
    // 2. Check context-specific questions
    contextComplete := cc.CheckContextQuestions(answers)
    
    // 3. Check configuration generation
    configGenerated := cc.CheckConfigurationGenerated()
    
    // 4. Determine overall status
    status := cc.DetermineStatus(essentialComplete, contextComplete, configGenerated)
    
    return status, nil
}
```

## Use Cases

### Use Case 1: Standard Individual Project

**Initialization**:
```bash
zqk system init --interactive
# Answers: Individual project, single developer, etc.
# Status: INITIALIZED
```

### Use Case 2: Partnership (Code Owner + Feature Builder)

**Initialization**:
```bash
zqk system init --interactive
# Answers: Partnership, Code owner + Feature builder, Acme Corp, Beta Inc, etc.
# Status: INITIALIZED
# Authority Model: Hierarchical with approval
# Alignment Model: Partnership constraints
```

### Use Case 3: Non-Interactive from File

**Initialization**:
```bash
zqk system init --answer-file answers.yaml
# Status: INITIALIZED
```

## Benefits

1. **Seed Questions First**: Ensures system is properly configured before use
2. **Environment Detection**: Answers determine operating environment automatically
3. **Partnership Support**: Handles complex organizational relationships
4. **Progressive Disclosure**: Start essential, add context-specific questions
5. **Answer Interpretation**: System interprets answers to configure appropriately
6. **Initialization Completion**: Clear criteria for when system is ready

## Related Documentation

- [Project Discovery and Strategic Alignment](./project-discovery-and-strategic-alignment-v1.0.md)
- [Role Discovery and Progressive Configuration](./role-discovery-and-progressive-configuration-v1.0.md)
- [Authority Resolution and Collaboration](./authority-resolution-and-collaboration-v1.0.md)

---

*This system ensures zqk is properly initialized with seed questions answered, environment detected, and partnership scenarios handled appropriately before the system can be considered operational.*

