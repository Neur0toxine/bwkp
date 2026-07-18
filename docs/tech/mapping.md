# Data mapping

The hierarchy is source-oriented: personal folders remain under Personal;
organization items are grouped by organization and primary collection; archive
and trash state are explicit groups. Source metadata that has no exact
KeePassXC representation is retained as protected `BW.*` attributes. This
includes secondary collection membership, URI matching rules, reprompt,
archive/trash timestamps, linked-field semantics, and passkey attributes that
KeePassXC does not model.

Native KeePassXC fields are used for title, username, password, URLs, notes,
TOTP, tags, passkey attributes, SSH agent settings, timestamps, history, and
attachments. Card, identity, license, passport, bank, and custom fields become
named protected or unprotected attributes as appropriate.

With `--append-source`, every entry also contains protected `BW.SourceJSON` and
the complete source identity metadata. This preserves the normalized decrypted
Bitwarden representation and permits future recovery. Without the flag, fields
that were converted exactly are not duplicated. Unknown item types fail the
export instead of being silently discarded.

Sends are outside the vault sync/export scope. Deleted and archived ciphers are
included. Attachment bytes never enter the JSON source field; they are added to
the KeePassXC attachment pool directly.
