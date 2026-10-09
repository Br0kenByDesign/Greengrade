# Build context: repository root. Purely static - no scripts, no backend.
FROM nginxinc/nginx-unprivileged:1.29-alpine
# Set via APP_ORIGIN and STATUS_URL in .env. Without STATUS_URL the status link is left out.
ARG APP_URL=https://app.example.com
ARG STATUS_URL=
# Public address of this landing page (LANDING_URL in .env), used for sitemap and link previews.
ARG SITE_URL=https://example.com
# Set when CONTACT_EMAIL is configured; otherwise the contact links are left out.
ARG CONTACT_FORM=
COPY landing/nginx.conf /etc/nginx/conf.d/default.conf
COPY landing/site/ /usr/share/nginx/html/
COPY design/fonts /usr/share/nginx/html/design/fonts
COPY design/icons /usr/share/nginx/html/design/icons
COPY design/fonts.css design/tokens.css design/favicon.svg design/logo-mark.svg design/status.css /usr/share/nginx/html/design/
# Your own pages (e.g. a filled-in impressum.html and datenschutz.html) go into landing/local/.
# The folder is ignored by git and overrides files from landing/site/.
COPY landing/ /tmp/landing/
USER root
RUN if [ -d /tmp/landing/local ]; then cp -r /tmp/landing/local/. /usr/share/nginx/html/; fi \
 && rm -rf /tmp/landing \
 && cd /usr/share/nginx/html \
 && sed -i "s#__APP_URL__#${APP_URL}#g; s#__SITE_URL__#${SITE_URL%/}#g" *.html robots.txt sitemap.xml \
 && if [ -n "${STATUS_URL}" ]; then sed -i "s#__STATUS_URL__#${STATUS_URL}#g" *.html; \
    else sed -i '/data-optional="status"/d' *.html; fi \
 && if [ -z "${CONTACT_FORM}" ]; then sed -i '/data-optional="contact"/d' *.html; fi \
 && find /usr/share/nginx/html -name .gitkeep -delete
USER 101
EXPOSE 8080
