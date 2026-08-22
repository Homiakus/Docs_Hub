# ADR 0003: Multi-Space Isolation and Granular RBAC

## Status
Accepted

## Context
Growing teams require logical partitions for confidential departments (e.g. Finance, HR, Core Engineering) with varying access levels (read-only, contributor, space administrator).

## Decision
We introduced:
1. **Organizational Spaces**: First-class database entities that scope documents, categories, and tags.
2. **Role Hierarchy**: Three core roles (`admin`, `editor`, `reader`) controlling permissions to create, edit, transition workflow states, and manage system users.

## Consequences
- Clean logical multi-tenancy without database fragmentation.
- Backward compatibility with unclassified documents while providing a foundation for future enterprise SAML/OIDC role mapping.
