FROM node:24

WORKDIR /home/node
COPY . /home/node
RUN mkdir -p /home/node/logs
RUN --mount=type=secret,id=npmrc,target=/root/.npmrc \
    npm ci; npm run build; npm prune --omit dev;

ENTRYPOINT [ "node", "dist/index.js" ]