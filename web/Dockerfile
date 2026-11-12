# syntax=docker/dockerfile:1
# Frontend microservice: nginx serves the static UI and proxies /api/ to the
# backend given by BACKEND_URL (scheme://host:port, no trailing slash).
FROM nginxinc/nginx-unprivileged:1.27-alpine
ENV BACKEND_URL=http://api:8080
COPY nginx/default.conf.template /etc/nginx/templates/default.conf.template
COPY static/ /usr/share/nginx/html/
EXPOSE 8080