# Translation engine (BLI-764)

Translates imported ontology/schema formats (RDF/OWL, JSON Schema, etc.) into zqk-domain structures for traceability and downstream use.

## Interface and registry (CRIT-7641)

- **Translator**: `SourceFormat()` returns format key; `Translate(raw, opts)` returns `TranslateResult` (objects + warnings).
- **Registry**: `Register(t Translator)`; `Get(formatKey) Translator`; `TranslateByFormat(formatKey, raw, opts)` runs the translator for that format.

## Implemented translators (CRIT-7642)

- **RDF/OWL** (and aliases: `turtle`, `rdf_xml`, `jsonld`): produces one `domain_registry` TranslatedObject with a `domains` list.
  - **Turtle**: parses Turtle for `owl:Class`, `rdfs:label`, `rdfs:comment`, and `rdfs:subClassOf`.
  - **JSON-LD**: parses JSON-LD (when input looks like JSON) for `@type` owl:Class, `rdfs:label`, `rdfs:comment`, and `rdfs:subClassOf`; supports `@graph` and single-object forms.
  - Each domain entry includes `id`, `namespace`, and when present: `title` (from rdfs:label), `description` (from rdfs:comment), `sub_class_of` (parent class local name). ID placeholder `DOMAIN-REG-000` is replaced by the import pipeline with the next `DOMAIN-REG-NNN`.
  - **RDF/XML** (BLI-712): parses RDF/XML for `<owl:Class rdf:about="...">` with `rdfs:label`, `rdfs:comment`, and `rdfs:subClassOf rdf:resource`; produces the same domain_registry and spec-like output as Turtle/JSON-LD.

## Object-spec-like output

For each owl:Class (Turtle or JSON-LD), the translator also produces an **object_spec-like** structure in `TranslateResult.Specs`: `schema_version`, `ontology` (class local name), `extends` (parent class or `extensible_object`), `visibility`, `description` (from rdfs:comment), `traits`, `fields` (empty stub). Callers can use these for downstream spec generation (e.g. writing `docs/architecture/_internal/object_specs/<ontology>.yaml` via tooling).

## Pipeline (CRIT-7643)

The ontology import command (`zqk ontology import --file <path>`) invokes the translation engine after creating an `import_tracking` record: it calls `TranslateByFormat(formatKey, data, opts)`, updates the import_tracking status to `translated`, **persists** each translated object via storage (no direct YAML), reports object/warning counts, spec count (when present), and persisted count.
