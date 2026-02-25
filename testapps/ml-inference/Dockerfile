FROM python:3.10

ENV PYTHONDONTWRITEBYTECODE=1

WORKDIR /app

RUN pip install --no-cache-dir numpy scikit-learn

COPY script.py .

CMD ["python", "script.py"]
