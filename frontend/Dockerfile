FROM node:20-alpine AS base
WORKDIR /app
COPY package.json ./
RUN npm install

# ─── Développement ────────────────────────────────────────────────────────────
FROM base AS development
COPY . .
EXPOSE 5173
CMD ["npm", "run", "dev", "--", "--host", "0.0.0.0"]

# ─── Build production ─────────────────────────────────────────────────────────
FROM base AS build
COPY . .
RUN npm run build

FROM nginx:alpine AS production
COPY --from=build /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
