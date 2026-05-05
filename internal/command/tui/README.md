# internal/command/tui

Interactive terminal prompts for the tingly-box CLI. A small, self-contained
package: a handful of single-shot prompts plus a generic Wizard runner that
strings them together with a shared header, breadcrumb, and help line.

---

## Why this package exists here

The original code lived at `internal/tui/`. That location implies a
shared facility — something used by many commands across the binary. In
practice it was a leaf: only `internal/command/quickstart.go` ever imported
it. Moving it under `internal/command/tui` makes the dependency explicit in
the directory tree and prevents other commands from accidentally coupling to it.

The package is still a leaf. To avoid an import cycle (`internal/command/tui`
→ `internal/command` → `internal/command/tui`), the `QuickstartManager`
interface is defined here rather than in `internal/command`. `AppManager`
satisfies the interface implicitly — no adapter needed.

---

## What was wrong with the old version

The old `internal/tui` split across four subpackages (`components/`, `styles/`,
`wizards/`, and the root `tea.go`). In practice each subpackage had exactly one
file, so the hierarchy added navigation friction with no cohesion benefit.

Worse, the runtime behaviour was jarring:

| concern | old approach | effect |
|---|---|---|
| between-step headers | `fmt.Println` | plain text interleaved with tea output |
| list picker | `bubbles/list` | title bar, status bar, filter chrome, pagination all visible at once |
| colour | hard-coded hex, no dark/light switch | invisible on some terminals |
| help text | inconsistent per component | users had to guess keys |
| breadcrumb | dot row `● ● ○ ○` | no step names, no sense of progress |
| trace | tea wipes output on quit | prior answers vanish |
| `--tui` flag | wired but call commented out | flag did nothing |

Every prompt launched its own `tea.NewProgram`, so the header had to be
re-printed with `fmt.Println` before each one. The seams were visible.

---

## Architecture

```
internal/command/tui/
  tui.go          core types, shared theme, run() helper
  prompts.go      Confirm, Input, Select, MultiSelect
  wizard.go       Step[S], RunWizard[S], WithSpinner[T]
  quickstart.go   Quickstart wizard — the only consumer
```

Everything is in one flat package. There are no subpackages.

### Generic types

```go
// Result wraps any prompt value with how the prompt was dismissed.
type Result[T any] struct {
    Value  T
    Action Action  // ActionConfirm | ActionBack | ActionCancel
}

// Step is one phase in a wizard.
type Step[S any] struct {
    Name    string
    Skip    func(state S) bool
    Execute func(ctx StepContext, state S) (S, StepResult, error)
}
```

Go generics let the wizard carry arbitrary application state `S` without
interface boxing or type assertions. Each step receives the current state,
returns a (possibly mutated) copy, and signals what the wizard should do next
(`StepContinue`, `StepBack`, `StepDone`, `StepSkip`, `StepCancel`).

### Back navigation

The wizard keeps a `history []int` stack of step indices. `advance()` pushes
the current index before moving forward; `retreat()` pops it. This means
"back" always returns to wherever the user actually came from — even when some
steps were auto-skipped — with no special-casing needed.

### Header passing

Every prompt option struct has a `Header string` field. The wizard renders its
breadcrumb string once per step and passes it down:

```go
ctx := StepContext{Header: w.renderHeader(), ...}
newState, result, err := step.Execute(ctx, w.state)
```

Inside the prompt's `View()`, `Header` is printed above the question. Because
it's part of the same tea program (not a preceding `fmt.Println`), it stays
perfectly aligned with the rest of the output and is erased cleanly when the
prompt resolves.

### Scrollback trace

After a prompt resolves, its `View()` returns a single summary line instead of
the full interactive UI:

```
✓ Pick a provider: OpenAI
```

Because bubbletea clears only the last rendered frame, this line stays visible
in the terminal scrollback. Users can scroll up and see every answer they gave,
which is especially useful when the wizard has many steps.

---

## Prompts

### Confirm

Two visible buttons (`Yes` / `No`) that the user can toggle with `←`/`→`,
`Tab`, or `y`/`n`. `Enter` confirms the highlighted button. This is less
surprising than a raw `y/n` prompt because the current selection is always
visible.

### Input

Wraps `github.com/charmbracelet/bubbles/textinput` with an optional
`Validate func(string) error`. Validation runs on every keystroke and displays
an inline error below the field — no submit-and-fail cycle.

### Select

A flat list with a `❯` cursor. No title bar, no status bar, no filter chrome
from `bubbles/list`.

**Instant filter (fzf-style):** typing any character narrows the list in
real-time. A `filter:` hint appears above the list when active. `Esc` clears
the filter (first press) or triggers back navigation (second press with empty
filter). This two-stage Esc behaviour gives users a safe escape hatch without
accidentally leaving the prompt.

The filter is implemented by maintaining a `visible []int` slice — indices
into the immutable `items` slice that pass the current query (case-insensitive
substring). Moving the cursor moves through `visible`, not `items` directly.

**Key conflict avoidance:** `bubbles/key.Matches` checks key names, so binding
`"k"` to navigate-up would intercept `k` typed as a filter character.
`selectModel.Update` therefore uses a `tea.KeyMsg.Type` switch for navigation
keys (`KeyUp`, `KeyDown`, `KeyEnter`, `KeyEsc`) and routes `tea.KeyRunes`
directly to the filter string. The `k`/`j` vim aliases are intentionally absent
from Select for this reason.

### MultiSelect

Like Select but with `◉`/`○` checkboxes. Space toggles the item under the
cursor. `Enter` confirms the full selection. `k`/`j` aliases are retained here
because MultiSelect has no freeform filter — every keypress is a navigation
command.

---

## Theme

All colours are `lipgloss.AdaptiveColor` with separate light/dark values:

```go
colAccent  = lipgloss.AdaptiveColor{Light: "#7C3AED", Dark: "#B4BEFE"}
colSuccess = lipgloss.AdaptiveColor{Light: "#16A34A", Dark: "#A6E3A1"}
colDanger  = lipgloss.AdaptiveColor{Light: "#DC2626", Dark: "#F38BA8"}
colText    = lipgloss.AdaptiveColor{Light: "#1F2937", Dark: "#CDD6F4"}
colMuted   = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#9099B0"}
colSubtle  = lipgloss.AdaptiveColor{Light: "#9CA3AF", Dark: "#585B70"}
```

The palette leans toward Catppuccin Mocha on dark terminals and a neutral
gray-purple on light ones. No colour is hard-coded outside `tui.go`.

### Help line

Every prompt renders a single footer line via `helpLine([2]string{key, action},
...)`. The format is uniform:

```
↑/↓ navigate  ↵ select  esc back  ^c quit
```

One function, one format, consistent across all prompts.

---

## Wizard UX principles

**Never auto-skip a step based on its own content.** If a provider template
supports only one API style, the API Style step still appears — it just shows
context ("only style supported by X") and lets the user confirm or go back.
Auto-skipping hides information and breaks the back-navigation mental model:
users wonder why a step they passed through isn't reachable via Esc.

**Provider before API Style.** The original flow asked users to pick an API
style first, then a provider filtered to that style. This forced users to know
upfront whether a provider uses the OpenAI or Anthropic wire protocol — a
prerequisite that most users lack. The new flow shows all providers first
(description shows which styles each supports, e.g. `openai · anthropic`), then
asks for the API style in context of the chosen provider.

**Skip Welcome for returning users.** The Welcome step (`Skip: qsHasProviders`)
is only shown when no providers are configured. Returning users adding a second
provider land directly on the Credential step. This avoids the awkward
experience of reading onboarding text you've already seen.

**Breadcrumb shows named steps.** The header line rendered above every prompt:

```
Tingly Box · Quickstart   Step 3/6
✓ Welcome › ✓ Credential › ❯ Provider › · API Style › · Model › · Rules › · Done
```

`✓` means done, `❯` means active, `·` means upcoming. Step names are included
so users understand what's coming, not just how far along they are.

---

## Spinner

`WithSpinner[T](message, fn)` runs `fn` in a goroutine and renders an
animated `spinner.Points` spinner while it blocks. On completion:

- success → `✓ message` (green)
- failure → `✗ message` (red)

The spinner result line stays in scrollback like other prompt traces.

---

## Adding a new wizard step

1. Add a field to your state struct.
2. Write an `Execute` function with signature
   `func(ctx StepContext, state S) (S, StepResult, error)`.
3. Optionally write a `Skip` predicate `func(state S) bool`.
4. Append a `Step[S]{Name: "...", Execute: ..., Skip: ...}` to the slice
   passed to `RunWizard`.

No changes to the wizard runner are needed. The breadcrumb, step counter, and
back navigation all update automatically.
