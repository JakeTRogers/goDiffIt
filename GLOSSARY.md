# goDiffIt

goDiffIt compares two inputs as sets: each line is reduced to an Element, and the comparison ignores order and duplicates.

## Language

### Input

**Line**:
One line of raw input text, before Normalization.

**Element**:
The normalized string a Line yields, and the unit every Operation compares. It may be only part of the Line, such as its first column, a regex capture, or a hostname.
_Avoid_: value, key, entry, unique line

**Normalization**:
The reduction of a Line to an Element. A Line that normalizes to nothing contributes no Element.

**Case folding**:
The Normalization rule that makes comparison case-insensitive, on by default. It covers prefix and suffix trimming and extraction patterns too, so each matches its text in any case.

**Hostname stripping**:
The Normalization rule that reduces a fully qualified host name to its first label (`web01.example.com` becomes `web01`). An IP address is not a host name and stays whole.
_Avoid_: FQDN stripping, domain stripping

### Comparison

**Operation**:
The set comparison applied to A and B: Difference, Intersection, Union or Symmetric difference. Exactly one applies per run.

**Result**:
The Elements an Operation produces. The exit status reports whether the Result is empty (0) or non-empty (1), whatever the Operation.
_Avoid_: differences, diff output

**Difference**:
The default Operation: the Elements only in A (A−B) and the Elements only in B (B−A), reported as two separate Directions.
_Avoid_: diff

**Direction**:
One side of a Difference: A−B or B−A.

**Intersection**:
The Operation whose Result is the Elements present in both A and B.
_Avoid_: overlap, common lines

### Output

**Pipe mode**:
Output of bare Elements, one per line, with no headers. A Difference in Pipe mode keeps only A−B, so the output stays a single list.
_Avoid_: raw mode, quiet mode
