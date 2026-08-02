FROM python:3.13-alpine

WORKDIR /app

COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt

COPY . .

# Unprivileged: nothing here needs root.
RUN adduser -D -H parser
USER parser

CMD ["python", "-u", "main.py"]
