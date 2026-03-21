# AGENTS.md

## Project

This repository builds `home-guardian-ai`, a local AI video intelligence system for home, office, hotel, and factory environments.

## Core rule

Always prioritize **working MVP first** over broad but unfinished architecture.

If there is a choice between:

- a smaller feature that works now, and
- a larger design that is only partially implemented,

choose the smaller working feature first.

## Product direction

The system is intended to grow into these capability areas:

1. Detection
2. Identity
3. Behavior
4. Attendance / Presence
5. Vehicle / LPR
6. Access / Guest Mapping

However, not all of these need to be implemented in the first milestone.

## Current implementation priority

### First priority

Make the repository usable for a local pilot with:

- Docker Compose
- one RTSP camera
- person detection
- vehicle detection
- snapshot and event logging
- line crossing / zones
- basic alerts

### Second priority

Add lightweight registry and rule-based correlation:

- known people registry structure
- known vehicle registry structure
- basic known / unknown matching hooks
- event normalization

### Third priority

Add optional AI enrichments:

- face gallery
- attendance inference from identity + line crossing
- plate OCR / LPR

### Later priorities

- hotel guest/room mapping
- factory access workflows
- advanced behavior classification
- retraining pipeline
- model rollback UX

## Constraints

- Do not assume production-grade GPU/NPU acceleration on day one.
- Do not assume Linux-only deployment in the first dev pass.
- Keep the development path compatible with Windows host + Docker Desktop + WSL2.
- Avoid heavy always-on recording defaults.
- Prefer snapshots and short event clips first.
- Do not commit secrets, camera credentials, or personal data.

## What counts as done for the first real milestone

A task should be considered complete only if it helps achieve the following:

- stack starts locally
- one camera works
- person events appear
- vehicle events appear
- events can be filtered by zone or line crossing
- alerts can be triggered by simple rules
- documentation explains how to reproduce the setup

## Do not overbuild these in the first pass

Avoid adding these unless explicitly required by the task:

- multi-site orchestration
- distributed message buses
- full MLOps platform
- custom training dashboards
- hotel PMS integration
- ERP/WMS integration
- high-cardinality analytics databases
- full permission administration UI

## Implementation principles

### Keep layers separate

- Detection answers: what is visible?
- Identity answers: who is it?
- Behavior answers: what are they doing?
- Attendance answers: when did they enter or leave?
- Vehicle/LPR answers: what vehicle or plate is this?
- Access mapping answers: what is this person or vehicle allowed to do?

### Preserve upgrade paths

When adding files, schema, or services, design them so that future features can be added without breaking the MVP.

### Favor configuration over hardcoding

Use:

- config files
- JSON/YAML registries
- rule files
- environment variables

instead of hardcoded site-specific logic.

## File and docs expectations

If you add or change behavior, update the relevant docs:

- `README.md`
- `docs/MVP-FIRST.md`
- `docs/ARCHITECTURE.md`
- `docs/ROADMAP.md`
- `docs/DATA_MODELS.md`

## Review checklist

Before finishing any meaningful change, verify:

1. The change keeps the MVP path simple.
2. The change does not force future-only complexity into the current milestone.
3. The change is documented.
4. The change does not require real credentials in the repo.
5. The naming matches the current architecture docs.

## Preferred work style for coding agents

When asked to implement something:

1. inspect current repo structure
2. identify the smallest usable slice
3. implement the minimum coherent version
4. document the remaining gaps honestly
5. suggest the next 2–3 follow-up tasks

## Preferred terminology

Use these terms consistently:

- `identity` for person identity
- `behavior` for action or event classification
- `attendance` for check-in / check-out / presence
- `vehicle` for vehicle tracking and metadata
- `lpr` for license plate recognition
- `guest-mapping` for hotel guest/room linkage
- `access-control` for whitelist / blacklist / schedule / zone rules
- `registry` for local person/vehicle records
- `model-registry` for model version selection and rollback metadata
