# Spec feedback from the Go port

Places where spec 0.15.0 is silent, ambiguous, or written in the shape of
the Swift port. Each item: the section, the problem, what Swift does, what
Go does, and a proposal. Nothing here changes the spec by itself; the
proposals go through the spec repository's change flow.

## 1. §4.5.4: which references count as "used" by an interface

**Problem.** §4.5.4 puts into `clus` the CLUs referenced by "the `cluId` of
any state reference or action of its pages after the transformation". The
oracle `tools/oracles/omp.py` (`used_clus`) collects only component
`state`/`actions`, the `value` of the double widgets' sides, and the
members of `object`. It misses the channels of a `MULTISENSOR` (they are
not under `object`) and the `clickAction` of a `CONTACT_SENSOR_DOUBLE`
side. A CLU referenced only there is in `clus` by the text and missing by
the oracle. No case covers it, so both pass today.

**Go.** Follows the text: every `ValueRef` and `ActionRef` of the converted
pages.

**Proposal.** Keep the text, fix the oracle, and add a project fixture
with a CLU used only by a multisensor channel.

## 2. §4.5.1 item 5: an unpaired surrogate escape in node JSON

**Problem.** The list of errors (invalid UTF-8, `NaN`, out-of-range
numbers, repeated keys, depth) does not mention `"\ud800"`, an escape that
names half a surrogate pair. Python's `json` accepts it; a Go or Swift
string cannot hold it.

**Go.** Decodes it as U+FFFD, as `encoding/json` does.

**Proposal.** Say so: "an unpaired surrogate escape decodes as U+FFFD", or
make it `invalidData`. Either is testable with one node.

## 3. §4.5.1 item 4: which XML encodings a port must read

**Problem.** "The encoding is determined by the XML parser (the `encoding`
declaration, BOM)". Parsers differ in what they support: Go's
`encoding/xml` reads only UTF-8 unless the program supplies a charset
reader, Python's expat reads UTF-8, UTF-16 and Latin-1. The rule is clear
for a document with a DTD (always `invalidData`), but not for a valid
project in another encoding: one port imports it, another rejects it.

**Go.** UTF-8, and UTF-16 with a BOM (transcoded with the standard
library). Any other declared encoding is `invalidData`.

**Proposal.** Name the set: UTF-8 and UTF-16 must be read; any other
encoding is `invalidData`. Object Manager writes UTF-8.
