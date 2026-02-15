FROM python:3.10

ENV PYTHONDONTWRITEBYTECODE=1

WORKDIR /app

RUN pip install --no-cache-dir numpy scikit-learn

COPY app.py .

CMD ["python", "app.py"]
