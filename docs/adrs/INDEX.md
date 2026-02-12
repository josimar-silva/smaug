# Architecture Decision Records (ADRs)

This directory contains Architecture Decision Records (ADRs) for the Smaug project. ADRs document significant architectural decisions, their context, alternatives considered, and consequences.

## Overview

ADRs serve as a historical record of technical decisions and provide context for future maintainers. Each ADR follows a standardized format:

- **Status:** Draft, Proposed, Accepted, Deprecated, or Superseded
- **Date:** When the ADR was created
- **Context:** Why this decision was needed
- **Decision:** What was decided and why
- **Consequences:** Positive and negative impacts
- **References:** Related documents and resources

## Quick Navigation

| # | Title | Description | Status | Date |
|---|-------|-------------|--------|------|
| [001](./ADR-001-foundation.md) | Foundation Architecture | Core architecture, Gwaihir integration, deployment strategy, security model, observability setup | Draft (Refined) | 2026-02-11 |

## How to Read an ADR

1. **Start with Status and Date** - Understand when the decision was made
2. **Read Context** - Understand the problem being solved
3. **Review Decision** - See what was chosen and rationale
4. **Study Consequences** - Understand positive/negative impacts and risks
5. **Check References** - Find additional resources and related projects

## For New Decisions

When adding a new ADR:

1. Create a new file: `ADR-XXX-title.md`
2. Follow the template from existing ADRs
3. Keep it concise but thorough
4. Include code examples where helpful
5. Add to this INDEX.md
6. Link from relevant documentation

### ADR Template

```markdown
# ADR-XXX: Title

**Status:** Proposed (Draft|Proposed|Accepted|Deprecated|Superseded)
**Date:** YYYY-MM-DD
**Updated:** YYYY-MM-DD (if modified)

## Context

Why is this decision needed? What problem are we solving?

## Decision

What are we deciding? Why this approach?

## Alternatives Considered

What other options were evaluated?

## Consequences

### Positive
- Benefit 1
- Benefit 2

### Negative
- Drawback 1
- Drawback 2

### Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|-----------|
| Risk 1 | High | High | Mitigation strategy |

## References

- [Related Doc 1](link)
- [Related Project](link)
```

## ADR Guidelines

### When to Write an ADR

Write an ADR for decisions that:
- Have significant architectural impact
- Affect multiple components or layers
- Involve trade-offs between alternatives
- Will guide future development
- Are important for team understanding

### When NOT to Write an ADR

Skip an ADR for:
- Simple bug fixes
- Minor refactoring
- Implementation details
- One-off decisions with no long-term impact

### Decision Criteria

Use these questions to evaluate decisions:

1. **Scope:** Does this affect the overall architecture?
2. **Longevity:** Will this decision last months or years?
3. **Trade-offs:** Are there meaningful alternatives?
4. **Context:** Would future developers benefit from understanding why?
5. **Reversibility:** Is this decision reversible if needed?

## Status Legend

- **Draft:** Being written, not yet finalized
- **Proposed:** Ready for discussion and review
- **Accepted:** Decision is final and implemented
- **Deprecated:** No longer used, but kept for historical context
- **Superseded:** Replaced by a newer ADR

## Related Documentation

- [README.md](../../README.md) - User-facing project documentation
- [CONTRIBUTING.md](../../CONTRIBUTING.md) - Contribution process

---

**Last Updated:** 2026-02-12
