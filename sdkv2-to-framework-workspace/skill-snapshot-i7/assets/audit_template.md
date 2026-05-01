# SDKv2 → Framework Migration Audit

**Provider:** {{provider_name}}    **Audited:** {{date}}    **SDKv2 version:** {{sdk_version}}

## Summary

- Files audited: {{file_count}}
- Resources: {{resource_count}}
- Data sources: {{data_source_count}}
- `ForceNew` occurrences: {{force_new_count}}
- `ValidateFunc` / `ValidateDiagFunc`: {{validator_count}}
- `StateUpgraders` / `SchemaVersion`: {{state_upgrader_count}}
- `MaxItems: 1` candidates: {{max_items_1_count}}
- `MinItems > 0`: {{min_items_count}}
- Custom `Importer`: {{importer_count}}
- `Timeouts`: {{timeouts_count}}
- `Sensitive: true`: {{sensitive_count}}
- `DiffSuppressFunc`: {{diff_suppress_count}}
- `CustomizeDiff`: {{customize_diff_count}}
- `StateFunc`: {{state_func_count}}

## Per-file findings (top N by complexity)

| File | ForceNew | Validators | StateUpgraders | MaxItems:1 | NestedElem | Importer | CustomizeDiff | StateFunc |
|---|---|---|---|---|---|---|---|---|
| {{file_path}} | {{force_new}} | {{validators}} | {{state_upgraders}} | {{max_items_1}} | {{nested_elem}} | {{importer}} | {{customize_diff}} | {{state_func}} |

## Needs manual review

The audit uses semgrep for AST-aware multi-line matching, but the *decisions* — block vs nested attribute, single-step state-upgrade composition, composite-ID importer parsing — still need human/LLM judgment. Read every file in this list before proposing edits.

- {{file_path}} — {{reasons}}

## Next steps

1. Read every file listed under "Needs manual review".
2. Populate `checklist_template.md` from this audit (one block per resource).
3. Confirm scope with the user before starting workflow step 1.
