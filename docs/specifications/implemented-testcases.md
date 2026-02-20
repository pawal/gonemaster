# Implemented Gonemaster Testcases

This document is the authoritative inventory of currently implemented gonemaster testcases.

Source of truth used for this inventory:
- `engine/plan.go` (`moduleTestcases`, `moduleOrder`)
- `engine/engine.go` (`basicTests`, `syntaxTests`, `addressTests`, `connectivityTests`, `consistencyTests`, `delegationTests`, `dnssecTests`, `nameserverTests`, `zoneTests`)

Notes:
- DNSSEC testcase ordering is numeric (`dnssec01` ... `dnssec18`) as returned by `dnssecTestcaseNames()`.
- `dnssec12` is currently not implemented and therefore not present in this inventory.

## Summary
- Modules: 9
- Implemented testcases: 73

## Regeneration

```sh
make spec-export-implemented
```

## Module Inventory

### basic (3)
- basic01
- basic02
- basic03

### address (3)
- address01
- address02
- address03

### connectivity (4)
- connectivity01
- connectivity02
- connectivity03
- connectivity04

### consistency (6)
- consistency01
- consistency02
- consistency03
- consistency04
- consistency05
- consistency06

### delegation (7)
- delegation01
- delegation02
- delegation03
- delegation04
- delegation05
- delegation06
- delegation07

### dnssec (17)
- dnssec01
- dnssec02
- dnssec03
- dnssec04
- dnssec05
- dnssec06
- dnssec07
- dnssec08
- dnssec09
- dnssec10
- dnssec11
- dnssec13
- dnssec14
- dnssec15
- dnssec16
- dnssec17
- dnssec18

### nameserver (14)
- nameserver01
- nameserver02
- nameserver03
- nameserver04
- nameserver05
- nameserver06
- nameserver07
- nameserver08
- nameserver09
- nameserver10
- nameserver11
- nameserver12
- nameserver13
- nameserver15

### syntax (8)
- syntax01
- syntax02
- syntax03
- syntax04
- syntax05
- syntax06
- syntax07
- syntax08

### zone (11)
- zone01
- zone02
- zone03
- zone04
- zone05
- zone06
- zone07
- zone08
- zone09
- zone10
- zone11
