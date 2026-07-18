# Data mapping

The hierarchy is source-oriented: personal folders remain under Personal;
organization items are grouped by organization and primary collection; archive
and trash state are explicit groups. Multi-collection membership and source
identifiers are retained as protected metadata.

Native KeePassXC fields are used for title, username, password, URLs, notes,
TOTP, tags, passkey attributes, SSH agent settings, timestamps, history, and
attachments. Card, identity, license, passport, bank, and custom fields become
named protected or unprotected attributes as appropriate.

Every entry also contains protected `BW.SourceJSON` plus protected source
metadata. This preserves the normalized decrypted Bitwarden representation for
fields without a first-class KeePassXC equivalent and permits future recovery.
Unknown item types fail the export instead of being silently discarded.

Sends are outside the vault sync/export scope. Deleted and archived ciphers are
included. Attachment bytes never enter the JSON source field; they are added to
the KeePassXC attachment pool directly.
