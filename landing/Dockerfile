# Build context: repository root. Purely static - no scripts, no backend.
FROM nginxinc/nginx-unprivileged:1.29-alpine
# Address of your app, e.g. https://app.example.com (set via APP_ORIGIN in .env)
ARG APP_URL=https://app.example.com
COPY landing/nginx.conf /etc/nginx/conf.d/default.conf
COPY landing/site/ /usr/share/nginx/html/
COPY design/fonts /usr/share/nginx/html/design/fonts
COPY design/icons /usr/share/nginx/html/design/icons
COPY design/fonts.css design/tokens.css design/favicon.svg /usr/share/nginx/html/design/
USER root
RUN sed -i "s#__APP_URL__#${APP_URL}#g" /usr/share/nginx/html/*.html
USER 101
EXPOSE 8080
