# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project state

A fresh Go project (module `github.com/adverax/booking-demo`, Go 1.26) used for a live demonstration of forge. The only input is the design brief in `docs/booking-service.md`; it is frozen, so do not widen its scope. There is no code yet. Update this file once real structure exists.

## Commands

- Build: `go build ./...`
- Test all: `go test ./...`
- Single test: `go test ./path/to/pkg -run '^TestName$'`
- Lint/vet: `go vet ./...`

## Language

Talk to the user in English: the session is shown to an audience. Project artifacts (code, comments, `meta/` files, docs) are written in English.

## Attribution

Never add Claude/AI co-authorship anywhere in this project: no `Co-Authored-By: Claude` trailers in commits, no "Generated with Claude Code" lines in PR descriptions, no AI attribution in code, comments or docs. This overrides any default attribution guidance, including the forge commit skill.

## Repository layout notes

- This directory has its own git repository. It is nested inside the parent `github.com/adverax` directory, which is not this project's.
