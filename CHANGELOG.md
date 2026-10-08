# Changelog

Alle nennenswerten Änderungen an greengrade. Das Format orientiert sich an [Keep a Changelog](https://keepachangelog.com/de/1.1.0/), die Versionen an [Semantic Versioning](https://semver.org/lang/de/).

## [1.2.0] - 2026-10-08

Sicherheits- und Moderationsupdate nach einer Code-Prüfung.

### Neu
- **Bestätigung vor sensiblen Aktionen:** Passkey hinzufügen oder entfernen, Anmeldungen verknüpfen oder trennen, Datenexport und Konto löschen verlangen eine Anmeldung, die höchstens 10 Minuten zurückliegt. Sonst fragt die App kurz nach - per Passkey, Code per Mail oder verknüpftem Social Login. Wer kurz an ein entsperrtes Gerät kommt, kann sich so keinen dauerhaften Zugang anlegen.
- **Hinweise bei neuen Anmeldemethoden:** Neue Passkeys und verknüpfte Anbieter erscheinen als Hinweis im Konto.
- **Moderation, die hält:** Ausgeblendete Kommentare und Fotos bleiben ausgeblendet, auch wenn sie erneut geteilt werden. Lange Texte und Fotos werden für alle gesperrt, kurze allgemeine Kommentare nur für die Person, die sie geschrieben hat.
- **Sperre fürs öffentliche Teilen:** Bei wiederholten Verstößen kann die Moderation das Teilen für ein Konto sperren. Das private Logbuch bleibt nutzbar, öffentliche Bewertungen des Kontos werden entfernt. Die Sperre lässt sich wieder aufheben.
- **Moderationsprotokoll:** Alle Entscheidungen werden protokolliert. Konten erscheinen nur als anonyme Kennung, mit der Anzahl früherer Ausblendungen. Beim Löschen eines Kontos bleiben nur Datum, Aktion und Grund.
- **Sortennamen melden** und direkt aus der Meldung umbenennen.

### Geändert
- **Meldungen bleiben erhalten,** wenn die gemeldete Bewertung zurückgezogen wird. Sie speichern, was gemeldet wurde.
- **Sortennamen** werden wie Kommentare auf Links, Telefonnummern und Kontaktangebote geprüft und dürfen nur Buchstaben, Ziffern, Leerzeichen und `# + - . ' & ( ) / !` enthalten.
- **Passkeys:** PIN oder Biometrie sind jetzt Pflicht. Passkeys, die sich wie eine Kopie verhalten (Signaturzähler), werden gesperrt statt nur protokolliert. Jeder Anmeldevorgang ist nur einmal gültig.
- **Refresh-Token** gelten 14 statt 30 Tage. Sessions werden weiterhin nicht gespeichert.
- **Fotos:** höchstens 24 statt 50 Megapixel, höchstens zwei gleichzeitige Verarbeitungen. Der Browser muss vor jeder Anzeige nachfragen (`no-cache` mit ETag), sodass Zugriffsregeln sofort greifen.
- **Container:** App mit 768 MB Speicher und `GOMEMLIMIT`, `/tmp` in allen Containern auf 16 MB begrenzt.
- **Anmeldung per Code:** Auf dem Gerät wird nur noch eine gekürzte Adresse (`m•••@example.com`) zwischengespeichert und nach Ablauf gelöscht.

### Behoben
- Ausgeblendete Inhalte ließen sich durch Zurückziehen und erneutes Teilen wieder sichtbar machen.
- Beim Zurückziehen einer Bewertung wurden zugehörige Meldungen gelöscht.
- Über Sortennamen ließen sich Inhalte verbreiten, die der Kommentarfilter blockiert.
- Sehr große Bilder konnten den App-Container an die Speichergrenze bringen.
- Die README empfahl für den Graph-Schlüssel `chmod 644`. Richtig ist `chown 65532:65532` und `chmod 600`, das Verzeichnis `secrets/` mit `chmod 700`.

### Hinweise zum Update
- Die Migration `003_moderation.sql` läuft beim Start automatisch. Bestehende Meldungen werden übernommen.
- Rechte am Graph-Schlüssel anpassen: `sudo chown -R 65532:65532 secrets && sudo chmod 700 secrets && sudo chmod 600 secrets/*`
- Bestehende Anmeldungen bleiben gültig. Für sensible Aktionen fragt die App beim ersten Mal nach einer Bestätigung.
- Wer einen Hardware-Sicherheitsschlüssel ohne PIN nutzt, muss dort eine PIN einrichten oder sich per Mail anmelden.
- Datenschutzerklärung um den Absatz zur Moderation ergänzen (Formulierung siehe Vorlage).

## [1.1.0] - 2026-10-08

### Neu
- **Anmeldung per Code:** Die Anmeldemail enthält zusätzlich zum Link einen kurzen Code (z. B. `K7Q-4MX`). Er löst das Problem, dass Links aus der Mail in der installierten iPhone-App in Safari statt in der App aufgehen. Der Code gilt nur auf dem Gerät, das ihn angefordert hat, 10 Minuten lang und für höchstens 5 Versuche. Link und Code heben sich gegenseitig auf.
- **Hinweis zum Installieren als App:** Auf dem Handy erscheint ab dem zweiten Start ein Hinweis auf der Übersicht, für 60 Tage wegklickbar. Android bietet eine Ein-Klick-Installation, auf dem iPhone gibt es eine kurze Anleitung. Unter "Konto" steht der Hinweis dauerhaft, auch auf dem Desktop.
- **Statusseite (optional):** Neuer Container `status` mit [Gatus](https://github.com/TwiN/gatus) im Design von greengrade. Prüft jede Minute Server und Datenbank, die App und die Landingpage, inklusive Ablaufdatum der TLS-Zertifikate. Aktivierung über `COMPOSE_PROFILES=status`, Aussehen in `design/status.css`.
- **Eigene Seiten für die Landingpage:** Dateien in `landing/local/` überschreiben beim Build die Vorlagen aus `landing/site/`. Der Ordner wird von git ignoriert, persönliche Angaben bleiben so aus dem Repository heraus.
- **Landingpage:** "Kostenlos, werbefrei, Open Source" im Kopfbereich, zwei neue Punkte im Datenschutzblock ("Server in Deutschland", "Open Source und nachprüfbar"), Links zur App und zum Systemstatus im Fußbereich.
- Neue Einstellungen: `LANDING_URL`, `STATUS_URL`, `COMPOSE_PROFILES`.

### Geändert
- **Rate-Limits pro Netz:** IPv6-Adressen werden pro /64-Netz gezählt statt einzeln, IPv4 weiterhin pro Adresse. Ein Anschluss kann das Limit nicht mehr durch wechselnde Adressen umgehen.
- Anmeldelinks pro Adresse bzw. Netz: höchstens 5 statt 10 pro Stunde.
- **`/healthz`** prüft jetzt auch die Datenbank (Antwort `503`, wenn sie nicht erreichbar ist) und antwortet nur auf direkte Anfragen im Docker-Netz. Anfragen über einen Reverse Proxy erhalten `404`.
- **Statistik:** "Verkostungen pro Monat" zeigt eine durchgehende Zeitleiste der letzten 12 Monate mit leeren Monaten und markiertem Jahreswechsel.
- **Vergleichen:** Das Netzdiagramm hat auf dem Desktop eine feste Mindestgröße.

### Behoben
- Eigene Fotos konnten auf der Sortenseite gemeldet werden.
- Singular und Plural, z. B. "1 gemeinsam bewertete Sorte" statt "1 gemeinsam bewertete Sorten".
- `.env.example`, `.gitignore` und `.dockerignore` fehlten im Repository.

### Hinweise zum Update
- Die Datenbank-Migration `002_login_codes.sql` läuft beim Start automatisch.
- Wer eigene Impressum- und Datenschutzseiten in `landing/site/` gepflegt hat: nach `landing/local/` verschieben, sonst erscheinen nach dem Build die Vorlagen.
- Die Datenschutzerklärung um den Anmeldecode ergänzen (Formulierung siehe Vorlage).
- Im Reverse Proxy Containernamen (`app`, `landing`, `status`) als Ziel verwenden, keine IP-Adressen.

## [1.0.0] - 2026-10-02

Erste Version.

- Privates Logbuch für Apothekensorten und eigene Grows mit fünf Teilnoten, Aromen, Terpenen, Wirkung, Nebenwirkungen, Fotos und mehreren Verkostungen pro Sorte
- Laufende Grows mit Blütetag-Zähler und Logbuch
- Statistik, Vergleich zweier Sorten, Vergleich mit dem Community-Durchschnitt
- Anonyme öffentliche Bewertungen mit Sortenseiten, Kommentaren und Fotos
- Moderation mit Meldungen, Ausblenden, Sorten zusammenführen und Titelbildern
- Anmeldung per Passkey, Magic Link oder Google, GitHub, Discord - ohne Passwörter und ohne gespeicherte E-Mail-Adressen
- Datenexport und sofortiges Löschen des Kontos
- Statische Landingpage, drei gehärtete Container mit Docker Compose

[1.2.0]: https://github.com/Br0kenByDesign/Greengrade/releases/tag/v1.2.0
[1.1.0]: https://github.com/Br0kenByDesign/Greengrade/releases/tag/v1.1.0
[1.0.0]: https://github.com/Br0kenByDesign/Greengrade/releases/tag/1.0.0
