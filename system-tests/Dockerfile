FROM mcr.microsoft.com/playwright:v1.61.0-noble

WORKDIR /work

COPY package.json package-lock.json* ./
RUN if [ -f package-lock.json ]; then npm ci; else npm install; fi

COPY playwright.config.ts ./
COPY specs ./specs
COPY testpdf.pdf ./

CMD ["npm", "run", "test:system"]
