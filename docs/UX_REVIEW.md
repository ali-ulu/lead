# UX Review

Date: 2026-09-21

This review focuses on the operating flow of Lead Hunter rather than decorative styling.

## Problems found in the first UI

1. **Too many competing layers on one screen.**
   The sidebar, oversized marketing headline, search form, filters, four metric cards, results list and persistent detail pane all competed for attention.

2. **The main job was not visually dominant.**
   The product exists to search a market, scan opportunities and inspect one lead. The first UI treated all secondary features as equally important.

3. **The Pipeline navigation was misleading.**
   It behaved mostly like another filter rather than a distinct workflow, so the sidebar added navigation cost without adding a meaningful destination.

4. **The permanent right detail pane wasted space.**
   Before a lead was selected, almost half the workspace was an empty panel. After selection, audit, scoring, pipeline and outreach were stacked into one long surface.

5. **Too many cards, pills and badges.**
   Contact channels, pipeline state, website state, score and metrics created badge noise. This made scanning slower instead of faster.

6. **Website state language was too confident.**
   Open data missing a website field is not proof that no website exists. The UI now says “No site found” and distinguishes unaudited sites as “Not checked.”

7. **Mobile layout was not acceptable.**
   The original table/list structure and two-pane detail approach created unnecessary vertical and horizontal pressure on small screens.

8. **Keyboard access was incomplete.**
   Lead rows were mouse-oriented. Results can now be opened with Enter or Space and receive visible focus.

## New interaction model

The product now has three layers:

1. **Search**
   Country, city/area, industry and radius.

2. **Results**
   A compact sortable-by-score list with only the information needed to decide what to inspect:
   business, website state, contact availability, pipeline stage and opportunity score.

3. **Lead drawer**
   Selecting a lead opens a temporary right-side drawer instead of permanently consuming half the screen.

   The drawer is split into:
   - Overview
   - Website
   - Outreach

Secondary work is hidden until the user asks for it.

## Visual decisions

- Removed the sidebar.
- Removed the oversized landing-page headline.
- Removed the four metric cards.
- Replaced card-heavy lead rows with a scanning table.
- Replaced the permanent detail panel with a drawer.
- Replaced one long detail screen with tabs.
- Kept one accent color for priority and action.
- Reduced border, badge and label density.
- Preserved the dark AI-ULU visual language without turning the product into a generic AI dashboard.

## Additional fixes from review

- Mobile result table drops non-essential Contact and Stage columns instead of forcing horizontal overflow.
- Website audit redirects are revalidated so a public URL cannot silently redirect the auditor to a local/private target.
- Existing CI continues to check unit tests, Python compilation and JavaScript syntax.
