---
name: refresh-reservations
description: Pull the latest JV, Freshman, and Varsity baseball schedules from Arbiter Live and update field reservations in config.yaml. Use this when asked to refresh or update reservations.
---

Update `config.yaml` with the latest JV, Freshman, and Varsity field
reservations by pulling current schedules from Arbiter Live.

## Steps

1. **Fetch the JV schedule** from:
   https://www.arbiterlive.com/Teams/Schedule/608595?activeEntityId=18923

2. **Fetch the Freshman schedule** from:
   https://www.arbiterlive.com/Teams/Schedule/813723?activeEntityId=18923

3. **Fetch the Varsity schedule** from:
   https://www.arbiterlive.com/Teams/Schedule/1638657?activeEntityId=18923

4. **Extract home games** from each schedule:
   - For JV: find all games at **"Reading - Washington Park"**
   - For Freshman: find all games at **"Reading"** locations that map to
     **Symonds Field** (look for "Symonds" in the location)
   - For Varsity: find all home games (marked "vs", not "@") at
     **"Moscariello Field"** (look for "Moscariello" in the location).
     The location typically appears as "Reading Memorial HS /
     Moscariello Field at Morton Park".

5. **Replace reservations** in `config.yaml`:
   - Remove all existing reservations with `reason: "JV"` from the
     **Washington Park** field entry.
   - Remove all existing reservations with `reason: "Freshman"` from the
     **Symonds Field** entry.
   - Remove all existing reservations with `reason: "Varsity"` from the
     **Moscariello Ballpark** field entry.
   - Add a new reservation for each extracted home game date, using the
     format:
     ```yaml
     - date: "YYYY-MM-DD"
       reason: "JV" # or "Freshman" or "Varsity"
     ```
   - Dates should be in ascending order.
   - If no home games are found for a field, leave that field with no
     reservations (no `reservations:` key).

6. **Report what changed**: list any dates that were added or removed
   compared to the previous config so I can see what's new.

## Important notes

- The Arbiter schedule does not include the year in its dates. The current
  season year can be inferred from the `season.start_date` in `config.yaml`.
- Only add reservations for games at the relevant field — skip away games
  and games at other locations.
- Do not modify any other part of `config.yaml` (other fields, blackouts,
  rules, etc.).
