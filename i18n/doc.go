// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

/*
Package i18n implements message catalog translation for every user-facing
string in routercli: login prompts, command descriptions, and
runtime messages. A catalog is a flat YAML file, one per language, keyed
by a short language code taken from its filename, so var/lang/en.yaml
becomes "en".

This does not use golang.org/x/text/message. That package is built
around ICU-style plural and gender rules and compiled message catalogs,
more machinery than a CLI's short, mostly static strings need. A flat
map of key to string, with printf-style substitution, covers everything
a routercli message actually requires, stays trivial to read and
hand-edit, matching var/tree/tree_structure.yaml's own philosophy, and
keeps the dependency footprint down.

# Two Ways A String Can Reach The User

Every user-facing string in this project takes one of two forms. A
literal string, written directly in Go or in a tree YAML file's plain
desc field, is never translated. What is written is exactly what a user
sees, in every language. A catalog lookup, through Translator.T in Go,
or through a tree YAML file's desc_key, help_key, or arghelp_key field
instead of desc, help, or arghelp, resolves against whichever catalog is
currently active, see Catalog Lookup below.

A tree file entry can set both a literal field and its key counterpart,
see Command.ResolvedDesc in package command. When both are set, the key
wins whenever a Translator is available. The plain literal is used only
as a fallback if translation was never wired up at all, a nil
Translator, see T's own doc comment. Every command file this project
ships uses the key form throughout, to demonstrate the full translation
path end to end.

# Catalog Lookup

T looks up a key in the current language, falls back to the default
language, and beyond that falls back to the literal key itself, printed
in double brackets, for example "[[show.desc]]". Seeing that bracketed
form in a running CLI means exactly one thing: no catalog, in any loaded
language, has an entry for that key. Check var/lang/<code>.yaml for a
typo, or a line that was simply never added.

# The Cost Of A Translation Key

Using a key such as desc_key means a translated string exists in two
places at once: the key name itself, invented in the tree YAML file, and
the actual English text in var/lang/en.yaml, keyed by that same name.
Every other language's catalog then needs its own entry, keyed the same
way, with its own translated text. This is real overhead for the default
language specifically, since English gains nothing from being looked up
through a key rather than written literally.

The alternative, skipping the key and letting a catalog key its own
entries by the literal English text itself, avoids inventing a key name,
but does not remove the requirement that some catalog contain that exact
string for T to find. Without a key at all, ResolvedDesc never calls T
in the first place, see command.Command.ResolvedDesc, so a command with
only a plain desc field is never translatable no matter what another
catalog contains. There is no way to make English free and still have that
string be translatable later. The choice is between naming a key now, or
committing to the literal English text as the permanent key later. This
project uses named keys, show.desc rather than the literal sentence
itself, because a short, meaningful key survives an English wording
change without invalidating every other language's catalog entry.
Reword var/lang/en.yaml's show.desc line, and every other language's
show.desc entry is still correctly linked to the updated English
meaning. Keying by literal text would break that link the moment the
English wording changed at all.
*/
package i18n
