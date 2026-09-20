# Product Plan

## One-line product

A global B2B opportunity finder for local businesses that have no website or a weak website.

## Core job

Choose a country/region/city and business category. Discover candidates, normalize contact/web presence data, audit the website when present, calculate an explainable opportunity score, then prepare a tailored outreach draft.

## Workflow

1. Select market: country + city/area.
2. Select sector/category and radius.
3. Fetch candidates from open data.
4. Normalize and deduplicate.
5. Split into `missing`, `weak`, `healthy`, `unknown` web status.
6. Audit websites where present.
7. Score each candidate with reasons.
8. Human reviews lead detail.
9. Generate a personalized draft.
10. Move lead through pipeline states.

## Pipeline

`new -> reviewed -> contacted -> replied -> proposal -> won | lost | do_not_contact`

## Main filters

Country, city/region, category, website status, minimum opportunity score and pipeline state.

## Design decisions

- no blind mass email blasting
- no uncontrolled scraping
- no dependency on paid data sources for the base workflow
- no black-box AI scoring
- human review before outreach
