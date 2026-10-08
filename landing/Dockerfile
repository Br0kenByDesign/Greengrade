# Build context: repository root. Purely static - no scripts, no backend.
FROM nginxinc/nginx-unprivileged:1.29-alpine
# Set via APP_ORIGIN and STATUS_URL in .env. Without STATUS_URL the status link is left out.
ARG APP_URL=https://app.example.com
ARG STATUS_URL=
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
 && sed -i "s#__APP_URL__#${APP_URL}#g" /usr/share/nginx/html/*.html \
 && if [ -n "${STATUS_URL}" ]; then sed -i "s#__STATUS_URL__#${STATUS_URL}#g" /usr/share/nginx/html/*.html; \
    else sed -i '/data-optional="status"/d' /usr/share/nginx/html/*.html; fi \
 && find /usr/share/nginx/html -name .gitkeep -delete
USER 101
EXPOSE 8080
