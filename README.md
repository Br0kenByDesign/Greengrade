<p align="center"><img src="design/app-icon.svg" width="96" alt=""></p>

<h1 align="center">greengrade</h1>

<p align="center">Ein privates Logbuch zum Bewerten von Cannabis aus Apotheke und eigenem Grow.<br>Selbst gehostet, ohne Passwörter, mit optionalen anonymen öffentlichen Bewertungen.</p>

---

greengrade ist eine Web-App (PWA) für Erwachsene, die festhalten wollen, wie ihnen eine Sorte gefallen hat: Geruch, Geschmack, Aussehen, Wirkung und Qualität, dazu Aromen, Terpene und Nebenwirkungen. Alles bleibt privat, bis man eine Bewertung bewusst teilt. Geteilte Bewertungen fließen anonym in öffentliche Sortenseiten ein.

Die Oberfläche ist auf Deutsch.

 <img width="600" height="350" alt="Bildschirmfoto vom 2026-10-02 22-42-12" src="https://github.com/user-attachments/assets/b04b30c9-a0d9-44bb-9d10-3c453816d109" />


## Funktionen

**Logbuch**
- Einträge für Apothekensorten (Hersteller, Produkt, THC/CBD, Charge, Bestrahlung, Preis, Apotheke) und eigene Grows (Saatgut, Umgebung, Medium, Zeiten, Ertrag)
- Blüten und Extrakte, Einheiten passen sich an (% oder mg/ml)
- Fünf Teilnoten von 1 bis 10, Gesamtnote, Radar-Diagramm
- Aromen, Geschmack, Wirkungen, Nebenwirkungen, Terpene und Tageszeit als Tags, auch eigene Begriffe
- Konsumart, Vaporizer-Temperatur, Wirkungseintritt und Wirkdauer
- Mehrere Verkostungen pro Sorte (z. B. nach dem Curing erneut bewerten)
- Fotos mit automatischer Entfernung aller Metadaten
- Merkliste, Entwürfe auf dem Gerät, Suche und Filter

**Grows**
- Laufende Grows mit Blütetag-Zähler und voraussichtlichem Erntedatum
- Logbuch pro Grow, "Heute auf Blüte umgestellt" und "Heute geerntet" mit einem Klick
- Nach der Ernte direkt aus dem Grow heraus bewerten

**Auswertung**
- Statistik: Noten nach Kategorie, Verlauf pro Monat, Bestenliste, Grow vs. Apotheke, häufigste Aromen, Wirkungen und Terpene, Durchschnittspreis und -THC
- Zwei Sorten nebeneinander vergleichen
- Vergleich der eigenen Noten mit dem Community-Durchschnitt

<img width="600" height="350" alt="image" src="https://github.com/user-attachments/assets/80003fd7-6341-451c-ba3f-03ac6003fabe" />


**Öffentliche Bewertungen**
- Pro Eintrag wählbar: Bewertung öffentlich teilen, optional mit Kommentar und Foto
- Öffentlich sichtbar sind nur Noten, Kommentar, Foto und der Monat - nie Name, Herkunft oder Konsumdetails
- Sortenseiten mit Durchschnitt, Teilnoten, Verteilung, Kommentaren und Fotos
- Sortenvorschläge beim Tippen, damit Bewertungen derselben Sorte zusammenfinden
- Rangliste mit gewichtetem Durchschnitt (eine einzelne 10/10 landet nicht sofort oben)
- Für Sorten ohne Foto wird automatisch eine eigene Illustration erzeugt

<img width="600" height="350" alt="image" src="https://github.com/user-attachments/assets/2d828252-ef33-44c9-a595-4d8264ddcec0" />

**Moderation**
- Kommentare und Fotos melden
- Moderationsbereich: Meldungen prüfen, Inhalte ausblenden, Sorten umbenennen und zusammenführen, Titelbilder festlegen
- Betroffene werden in der App informiert (es gibt keine E-Mail-Adressen, an die man schreiben könnte)

**Konto**
- Anmeldung per Passkey, Magic Link per Mail oder optional Google, GitHub, Discord
- Mehrere Passkeys, verknüpfte Anmeldungen, "Überall abmelden"
- Datenexport als ZIP (JSON und Fotos), Konto sofort und vollständig löschen
- Hell, dunkel oder systemabhängig

**Landingpage**
- Statische Seite zur Erklärung der App, mit Vorlagen für Impressum und Datenschutz

<img width="600" height="360" alt="Bildschirmfoto vom 2026-10-02 23-06-16" src="https://github.com/user-attachments/assets/e11ee649-3746-4de3-bca3-44d6652ff6bf" />


## Datenschutz und Sicherheit

- **Keine Passwörter, keine Sessions in der Datenbank.** Zugang über kurzlebige, Ed25519-signierte JWTs in `__Host-`-Cookies (`HttpOnly`, `Secure`, `SameSite=Lax`). Access-Token 15 Minuten, Refresh-Token 30 Tage.
- **Keine E-Mail-Adressen in der Datenbank.** Gespeichert wird nur ein HMAC-SHA-256-Fingerabdruck mit geheimem Schlüssel.
- **Magic Links** sind 10 Minuten gültig und nur einmal verwendbar. Das Token steht im URL-Fragment und wird erst per Klick eingelöst, damit Mail-Scanner es nicht verbrauchen.
- **Passkeys** (WebAuthn): Gespeichert werden nur öffentliche Schlüssel.
- **Social Login** fragt nur die Nutzer-ID ab, keine E-Mail und kein Profil. Konten werden nie automatisch über E-Mail-Adressen verknüpft.
- **"Überall abmelden"** und Konto löschen wirken sofort.
- **Fotos** werden im Browser und auf dem Server neu kodiert. Standort, Aufnahmezeit, Kameradaten und alle anderen Metadaten verschwinden, präparierte Dateien werden neutralisiert. Fotos sind privat, solange sie nicht geteilt werden.
- **Missbrauchsschutz** ohne Drittanbieter: Proof-of-Work nach dem ALTCHA-Protokoll, Rate-Limits pro IP und pro E-Mail-Fingerabdruck, Filter gegen Links, Telefonnummern und Kontaktangebote in öffentlichen Kommentaren.
- **CSRF-Schutz** über SameSite-Cookies und `Origin`-Prüfung.
- **Strikte Content-Security-Policy** ohne Inline-Skripte. Schriften sind selbst gehostet, es gibt keine Anfragen an Dritte und kein Tracking.
- **Datensparsame Logs** ohne IP-Adressen, Query-Strings oder IDs.
- **Gehärtete Container:** Distroless-Image ohne Shell, non-root, read-only, `cap_drop: ALL`, `no-new-privileges`. Die Datenbank hängt in einem internen Netz ohne Internetzugang.

## Aufbau

| Container | Inhalt |
|---|---|
| `app` | Go-API mit eingebetteter Svelte-5-PWA |
| `db` | PostgreSQL 17 |
| `landing` | Statische Landingpage (nginx, keine Skripte, kein Backend) |

App und Landingpage laufen typischerweise unter zwei (Sub-)Domains, z. B. `app.example.com` und `example.com`.

## Schnellstart

Voraussetzungen: Docker mit Compose, eine Domain, ein Reverse Proxy mit TLS und ein Mailkonto zum Versenden.

```bash
git clone https://github.com/Br0kenByDesign/Greengrade.git greengrade
cd greengrade
cp .env.example .env
```

Secrets erzeugen und in `.env` eintragen:

```bash
openssl rand -base64 32   # APP_SECRET
openssl rand -base64 32   # EMAIL_PEPPER  - sicher aufbewahren, nie ändern!
openssl rand -hex 24      # POSTGRES_PASSWORD
```

Dann `APP_ORIGIN`, `RP_ID`, die Mail-Einstellungen und `ADMIN_EMAILS` setzen und starten:

```bash
docker network create proxy   # falls dein Reverse Proxy noch kein gemeinsames Netz hat
docker compose up -d --build
docker compose logs -f app
```

Zum Ausprobieren ohne Mailserver kann `MAIL_PROVIDER=log` gesetzt werden. Die Anmeldelinks erscheinen dann im Log. **Nicht im Produktivbetrieb verwenden.**

## Konfiguration

Alle Einstellungen stehen kommentiert in [`.env.example`](.env.example). Die wichtigsten:

| Variable | Bedeutung |
|---|---|
| `APP_ORIGIN` | Öffentliche Adresse der App, z. B. `https://app.example.com` |
| `RP_ID` | Passkey-Domain (App-Domain oder übergeordnete Domain). Nach dem Start nicht mehr ändern. |
| `APP_SECRET` | Signiert die Anmelde-Tokens. Ändern meldet alle ab. |
| `EMAIL_PEPPER` | Schlüssel für die E-Mail-Fingerabdrücke. Darf nie verloren gehen oder sich ändern. |
| `POSTGRES_PASSWORD` | Datenbankpasswort |
| `MAIL_PROVIDER` | `smtp`, `graph` oder `log` |
| `ADMIN_EMAILS` | Konten mit Zugriff auf die Moderation (kommagetrennt) |
| `MODERATION_EMAIL` | Erhält bei jeder Meldung eine kurze Benachrichtigung |
| `PRIVACY_URL`, `IMPRINT_URL` | Links zu Datenschutz und Impressum in der App |
| `OAUTH_*` | Zugangsdaten für Social Login, leer = deaktiviert |
| `TRUSTED_PROXIES` | Netze, deren `X-Forwarded-For` vertraut wird |

Secrets lassen sich auch als Datei übergeben: `APP_SECRET_FILE=/run/secrets/app_secret` usw.

## Reverse Proxy

`app` und `landing` lauschen jeweils auf Port 8080 im Docker-Netz `PROXY_NETWORK`. Der Reverse Proxy muss TLS terminieren und `X-Forwarded-For` setzen. Uploads bis 15 MB müssen erlaubt sein.

**Caddy**

```
example.com {
    reverse_proxy landing:8080
}
app.example.com {
    request_body {
        max_size 20MB
    }
    reverse_proxy app:8080
}
```

**nginx**

```nginx
server {
    server_name app.example.com;
    listen 443 ssl http2;
    # ssl_certificate ...
    client_max_body_size 20m;
    location / {
        proxy_pass http://app:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

**Nginx Proxy Manager / Traefik:** Je einen Host für die beiden Domains auf `app:8080` bzw. `landing:8080` anlegen, SSL erzwingen und bei der App das Upload-Limit auf 20 MB erhöhen (NPM: unter "Advanced" `client_max_body_size 20m;`).

Ohne Reverse Proxy im Docker-Netz kannst du in `docker-compose.yml` das Netz `proxy` entfernen und die Ports lokal veröffentlichen (siehe Kommentare dort).

## Mailversand

Ohne Mailversand kann sich niemand per Link anmelden. Damit die Mails nicht im Spam landen, sollten SPF, DKIM und DMARC für die Absenderdomain eingerichtet sein.

### SMTP

```env
MAIL_PROVIDER=smtp
MAIL_FROM=noreply@example.com
SMTP_HOST=smtp.example.com
SMTP_PORT=587          # STARTTLS, oder 465 für TLS
SMTP_USER=noreply@example.com
SMTP_PASSWORD=...
```

Unverschlüsselte Verbindungen werden abgelehnt.

### Microsoft 365 (Graph)

1. In Entra ID eine App registrieren und Tenant-ID und Client-ID notieren.
2. Ein Zertifikat erzeugen und in der App-Registrierung hochladen:
   ```bash
   mkdir -p secrets
   openssl req -x509 -newkey rsa:3072 -nodes -days 730 -subj "/CN=greengrade" \
     -keyout secrets/graph.key -out secrets/graph.crt
   chmod 644 secrets/graph.key secrets/graph.crt   # der Container läuft als UID 65532
   ```
   In `.env`: `GRAPH_CERT_FILE=/run/secrets/graph.crt` und `GRAPH_KEY_FILE=/run/secrets/graph.key`.
3. **Senderecht auf ein einziges Postfach beschränken.** Eine App mit `Mail.Send` darf sonst als *jedes* Postfach im Tenant senden. Empfohlen ist RBAC for Applications in Exchange Online (dann `Mail.Send` nicht zusätzlich in Entra vergeben):
   ```powershell
   Connect-ExchangeOnline
   New-ServicePrincipal -AppId <CLIENT-ID> -ObjectId <OBJECT-ID der Unternehmensanwendung> -DisplayName "greengrade"
   New-ManagementScope -Name "greengrade-sender" -RecipientRestrictionFilter "PrimarySmtpAddress -eq 'noreply@example.com'"
   New-ManagementRoleAssignment -App <CLIENT-ID> -Role "Application Mail.Send" -CustomResourceScope "greengrade-sender"
   Test-ServicePrincipalAuthorization -Identity <CLIENT-ID> -Resource noreply@example.com
   ```
   Alternativ: `Mail.Send` als Anwendungsberechtigung mit Admin-Zustimmung plus eine Application Access Policy (`New-ApplicationAccessPolicy ... -AccessRight RestrictAccess`).

greengrade sendet über Graph mit `saveToSentItems: false`, damit das Postfach keine Empfängeradressen sammelt.

## Social Login (optional)

Für jeden Anbieter eine OAuth-App anlegen, Redirect-URI: `<APP_ORIGIN>/api/auth/oauth/<anbieter>/callback`

| Anbieter | Wo | Scope |
|---|---|---|
| Google | Google Cloud Console → APIs & Dienste → Anmeldedaten → OAuth-Client "Webanwendung" | `openid` |
| GitHub | Settings → Developer settings → OAuth Apps | keiner |
| Discord | Developer Portal → Applications → OAuth2 | `identify` |

Client-ID und Secret in `.env` eintragen. Anbieter ohne Zugangsdaten werden nicht angezeigt.

## Erster Start

Mit einer Adresse aus `ADMIN_EMAILS` registrieren. In der Navigation erscheint dann "Moderation".

Vor einem öffentlichen Betrieb:

- `landing/site/impressum.html` und `landing/site/datenschutz.html` sind **Vorlagen**. Alle Angaben in eckigen Klammern ersetzen und rechtlich prüfen lassen. Die App kann Gesundheitsdaten verarbeiten (Art. 9 DSGVO); die Einwilligung wird bei der Registrierung abgefragt.
- `PRIVACY_URL` und `IMPRINT_URL` setzen.
- Die Landingpage verlinkt auf `APP_ORIGIN`; der Wert wird beim Build eingesetzt.

## Backups

```bash
docker compose exec db pg_dump -U greengrade greengrade | gzip > backup-$(date +%F).sql.gz
docker run --rm -v greengrade_photos:/data -v "$PWD":/b alpine tar czf /b/photos-$(date +%F).tgz -C /data .
```

Der Volume-Name hängt vom Projektnamen ab (`docker volume ls`). `EMAIL_PEPPER` gehört ebenfalls ins Backup - ohne ihn kann sich niemand mehr per Mail anmelden.

## Updates

```bash
git pull
docker compose up -d --build
```

Datenbank-Migrationen laufen beim Start automatisch.

## Entwicklung

Benötigt Go 1.26, Node.js 22 und ein laufendes PostgreSQL.

```bash
# Backend
cd app
export APP_ORIGIN=http://localhost:8080 DATA_DIR=./data MAIL_PROVIDER=log \
       DATABASE_URL=postgres://user:pass@localhost:5432/greengrade?sslmode=disable \
       APP_SECRET=$(openssl rand -base64 32) EMAIL_PEPPER=$(openssl rand -base64 32)
(cd web && npm install && npm run build)
go run .

# Frontend mit Hot Reload (zweites Terminal, leitet /api an :8080 weiter)
cd app/web && npm run dev
```

Projektstruktur:

```
design/                 Logo, Icons, Farben, Schriften (von App und Landingpage genutzt)
app/
  main.go
  internal/api/         HTTP-Handler
  internal/security/    Tokens, Proof-of-Work, Rate-Limits, Kommentarfilter
  internal/media/       Neukodierung von Fotos
  internal/mail/        SMTP, Microsoft Graph, Log
  internal/oauth/       Social Login
  internal/db/          Datenbank und Migrationen
  web/                  Svelte-5-PWA, wird nach webui/dist gebaut und eingebettet
landing/                Statische Landingpage und nginx-Konfiguration
```

## Sicherheitslücken

Bitte nicht als Issue, sondern privat melden - siehe [SECURITY.md](SECURITY.md).

## Hinweis

greengrade ist ein Werkzeug für Erwachsene zur persönlichen Dokumentation. Es ersetzt keine medizinische Beratung. Wer eine Instanz betreibt, ist selbst für den rechtskonformen Betrieb verantwortlich (Impressum, Datenschutz, Jugendschutz, Moderation nach dem Digital Services Act).

Schriften: Bricolage Grotesque und Geist, beide unter der SIL Open Font License (siehe `design/fonts`).
