# App Controller Skill

This skill helps AnyClaw drive local desktop applications with the tools that
already exist in the runtime.

## Focus

- Prefer high-level app workflows when they exist.
- Prefer target-based desktop tools over raw coordinates.
- Verify results after each meaningful UI action.

## Recommended Tool Order

1. `desktop_plan` for multi-step desktop tasks.
2. `desktop_resolve_target` and `desktop_activate_target` for stable UI targets.
3. `desktop_set_target_value` for typed input and form updates.
4. `desktop_screenshot`, `desktop_ocr`, `desktop_find_text`, and
   `desktop_verify_text` for visual fallback and verification.

## Notes

- This skill is guidance-only. It does not add a new executable entrypoint.
- It is intended to work alongside `vision-agent`.
