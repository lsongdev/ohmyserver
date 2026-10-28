# Development preferences

When redesigning a shared library, prefer the clearest context-aware public API over compatibility wrappers for old call shapes. Update dependent projects together when needed, and keep vendor-specific protocol details outside reusable transport/parsing packages. The user explicitly approved breaking old APIs in this situation.

# Interaction and forms

- Default to completing tasks in the page. Use modals only for decisions that need an immediate response or brief, contextual input.
- A modal should normally contain at most three simple fields and one primary action. This is a project review guideline, not an industry standard. Judge the task's complexity as well as its field count.
- Use dedicated, addressable pages for complex creation and configuration: grouped settings, dynamic lists, credentials, command/code editing, connection tests, previews, or workflows that require consulting other pages. CI repositories, scheduled jobs, and SSH hosts follow this rule.
- Use inline editing for frequent changes that benefit from seeing the surrounding list. A non-modal side panel can show quick details; a drawer that blocks the background has the same interruption cost as a modal. Do not move a long form into a narrow drawer.
- Keep short file operations (create, rename, destination selection) in small dialogs when appropriate. Text editing belongs in a dedicated workspace; image quick previews may use a dialog.
- Confirm actions according to their consequences: irreversible deletion, overwrites, credential replacement, or meaningful service disruption. Routine saves, refreshes, and tests do not need confirmation. Offer undo for reversible actions only when the underlying operation actually supports it.
- Show success with a lightweight notification, field errors beside their fields, and submission failures inline without clearing input. Long-running work uses visible progress and a task/detail page.
- Do not nest business-task modals. Warn about discarding changes only when changes exist. Protect input consistently across cancel, close, backdrop, Escape, navigation, and reload; prevent duplicate submissions.
- Dedicated editing pages must have stable URLs, work after refresh, and support browser Back. Save/cancel should return to the originating list or detail page while retaining available query/filter context.
- Page-level return links belong on the same row as the page title, aligned to the far right. Use the shared `Page` component's `back={{ href, label }}` prop; never place return navigation in a subtitle or mix it into action-button markup. Subtitles contain descriptions, paths, or status only. Keep title and return link together on narrow screens and let action buttons wrap onto another row.
- Retained dialogs must have an accessible name, sensible initial focus, focus containment, Escape handling, and restored focus on close. Prefer initial focus on Cancel for destructive confirmation. Avoid horizontal scrolling and long scrolling forms; do not shrink controls or enlarge dialogs just to fit a complex task.

# Build and service updates

- After each completed change and its relevant validation, rebuild and restart the running OhMyServer service so the user can inspect the current version. Browser assets are embedded in the Go binary; editing source alone does not update the running UI.
- Preserve the current listen address, data directory, and authentication settings. Shut down gracefully, start the rebuilt binary, and verify HTTP readiness and the updated page/assets before reporting completion.

References: [Carbon modal guidelines](https://www.carbondesignsystem.com/building-blocks/core/components/modal/guidelines), [W3C modal dialog pattern](https://www.w3.org/WAI/ARIA/apg/patterns/dialog-modal/).
