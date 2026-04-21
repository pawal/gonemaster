# Public UI explanations

Plain-language explanations for DNS findings shown on the public results
page. One file per module, authored for a domain owner who is not a DNS
operator.

The markdown files here are the source of truth. Sync them into the i18n
catalogs with:

```
node tools/i18n/sync-ui-explanations.mjs
```

The sync script:

- Writes every managed key to `ui-public/src/i18n/en.json`. Existing
  English values are overwritten from the markdown.
- Never touches other locales. The `t()` store falls back to English for
  missing keys, and `tools/i18n/missing-keys.sh` doubles as the
  translator work queue for whatever has not been localized yet.

## Key families

- `pub.tc_desc.<testcase>` - per-testcase plain-language description
  (1-3 sentences). Rendered under the testcase `<summary>` on the
  results page.
- `pub.tag.<module>.<TAG>.header` - short (3-6 words) human-readable
  summary of a single finding tag. Rendered inline before the templated
  engine message.
- `pub.tag.<module>.<TAG>.desc` - longer (1-3 sentences) description.
  Rendered inside a collapsible "About this finding" disclosure under
  each finding row.

Any of the three can be omitted; the UI degrades gracefully.

## File format

```markdown
# Public UI explanations: <MODULE>

## Testcase <testcase>

Description:

<one or more sentences describing the testcase>

## Tag <TAG>

Header: <short header>

Description:

<one or more sentences describing what the tag means to the user>
```

Blank lines separate blocks. The parser is forgiving about indentation
and trailing whitespace but not about the section-header shapes
(`## Testcase <id>` / `## Tag <TAG>`) - keep those exact.
