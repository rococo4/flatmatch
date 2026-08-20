FROM python:3.13-alpine

WORKDIR /app

COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt

COPY . .

# Unprivileged, with the same uid as the api image's nonroot user: both write
# into the shared photos volume, and its ownership is seeded from whichever
# image mounts it first.
RUN adduser -D -H -u 65532 parser && mkdir -p /data/photos && chown parser /data/photos
USER parser

CMD ["python", "-u", "main.py"]
