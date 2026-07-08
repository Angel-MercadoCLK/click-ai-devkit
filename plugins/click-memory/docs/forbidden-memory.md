# Forbidden Memory

This document mirrors the enforced categories in `internal/guard/patterns.yaml`. If a category is listed here, it must never be proposed for persistence.

## Forbidden categories

### PII

Never persist personally identifiable information.

The current enforced examples are:

- email addresses
- Argentine DNI-like numbers
- CUIT/CUIL identifiers
- phone numbers

Do not store raw values, partial values, or examples copied from real data.

### Amounts

Never persist monetary amounts.

The current enforced examples are:

- amounts with `$`
- amounts with `ARS`

Do not store reserves, premiums, payouts, balances, or any other money value.

### Policy numbers

Never persist policy numbers.

Current enforcement uses a **placeholder v0.1 format** while real Click formats are still pending confirmation. Treat all policy identifiers as forbidden now.

### Claim IDs

Never persist claim identifiers or siniestro identifiers.

Current enforcement uses a **placeholder v0.1 format** while real Click claim formats are still pending confirmation. Treat all claim identifiers as forbidden now.

### Customer identifiers

Never persist customer identifiers.

Current enforcement uses a **placeholder v0.1 format** while real Click customer identifier formats are still pending confirmation. Treat all customer identifiers as forbidden now.

## Deny-by-default rule

If a proposed entry might contain a forbidden category, do not save it.

## Source of truth

The definitive enforced categories live in `internal/guard/patterns.yaml`. This document must stay in sync with that file.
