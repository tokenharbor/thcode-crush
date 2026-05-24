List files and directories as a tree; skips hidden files and common system dirs.

LIMITS: Caps at {{ .MaxFiles }} entries per call. Real-world projects can have 100K+ files — `ls` is NOT for whole-project enumeration. If you receive a `[TOOL OUTPUT TRUNCATED]` banner, the directory has more entries than were returned; switch immediately to `glob` (pattern search) or `grep` (content search). NEVER assume the user's request was truncated just because the directory listing was. NEVER retry `ls` on the same path expecting a longer result.

WHEN TO PREFER: `glob '**/<name>'` to locate a known file/dir anywhere in the tree. `grep '<regex>' <path>` to find files containing specific text. Reserve `ls` for shallow checks of small, specific directories.
