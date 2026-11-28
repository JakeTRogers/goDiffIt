# Exit status reports a non-empty Result

goDiffIt exits 0 when the Result is empty, 1 when it holds at least one Element, and 2 on error, whatever the Operation. We chose this over "exit 1 means A and B differ as sets" so a script can test any Operation with one rule (e.g. "did the intersection find shared hosts?"). The cost is that `--union` of two non-empty inputs, even identical ones, always exits 1, so the name `DiffFoundError` ("differences found") understates what exit 1 means.
