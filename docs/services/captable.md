# Cap table

Route `/captable`, module id `captable`, accent `#126C5A`, migration `0331`, Go package `internal/captable`, web `web/app/src/pages/captable`, API `/api/v1/captable`.
A company's equity records: holders, share classes, option pools, securities (shares, options, warrants, SAFEs, notes), exercises, cancellations, transfers, a round modeller, an exit waterfall and exports.

## Who can open it (restricted)
The cap table is **not** open to all staff. Org admins have full access. Everyone else needs a row in Cap table › Access (admin only): **view** or **edit**. Anyone else gets `403` from every route and the page says it is restricted; the launcher tile is also governed by the app-access policies (`captable` is a business app, hidden from external users). The **Equity manager (cap table)** role preset gives a group the Cap table, Accounting, Contracts and Signatures apps; it does not by itself grant data access, so add each person under Access too. Every write, every export and every access change is audited under app `captable`.

## Maths (exact)
- Share counts are whole numbers (`BIGINT`). Money is whole paise (`BIGINT`). A price per share is `NUMERIC(24,8)` read back as text and handled as `big.Rat`. Rates (discount, interest, pool target) are integer basis points; multiples are thousandths. No float touches a result.
- **Percentages** are in units of 0.0001% (1,000,000 = 100%) and use the largest-remainder rule, so every column adds up to exactly 100%.
- **Vesting**: whole completed months since the vesting start (a month completes on the same day of the month, clamped to the month length), nothing before the cliff, then `floor(quantity × months / total)` stepping monthly, quarterly or yearly; the full quantity at the end.
- **Cap table on a date**: computed from securities and events up to that date. *Outstanding* counts issued shares; *fully diluted* adds outstanding options, warrants and the unallocated pool. SAFEs and notes are counted in money, not shares, until they convert. Cancelled options return to the pool; exercised and transferred ones stay drawn.
- **Convertibles**: a note's value is principal plus simple interest, `principal × rate × days / 365`, rounded half up. A SAFE or note converts at `min(round price, cap price, round price × (1 − discount))`; the cap price divides by the pre-round fully diluted count, or, for a **post-money SAFE**, by the company capitalisation after conversion, found by a 256-bit fixed-point iteration (refused when the SAFEs claim 95% or more).
- **Priced round**: `price = pre-money / (pre-round fully diluted + converting shares + pool top-up)`; pool top-up makes the unallocated pool the target share of the post-round count and lands before the round; new investor shares = `floor(investment × pre-round shares / pre-money)` in integers. Converting shares round down, the top-up to nearest.
- **Exit waterfall**: notes first, then SAFE money back, then preferred classes by seniority (equal seniority pro rata when short), then the rest per share to common, converted and participating preferred, and exercised options (exercise money joins the pot and is deducted). A non-participating class converts when that pays strictly more. Payouts use the largest-remainder rule so they add up to exactly the exit value, and are split to holders by shares.
- All of the above is covered by hand-computed tests in `internal/captable/captable_test.go` (e.g. a $500k post-money SAFE at a $5M cap converts into exactly 1,111,111 shares at 45 cents) and by an integration flow in `flow_test.go`.

## Links to other apps
Audit, the launcher and app-access policies, and the AI assistant. Round closing records the round, issues investor shares, converts SAFEs and notes and adds the pool top-up in one transaction. No data is sent to other apps.

## AI assistant
`pages/captable/ai/tools.ts`. Read-only: summary, holder, upcoming dates, round preview and exit preview. The assistant cannot change equity records and never calculates ownership itself.
