FROM python:3.9

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY . .
RUN if [ -f build.py ]; then python build.py; fi

CMD ["python", "app.py"]
