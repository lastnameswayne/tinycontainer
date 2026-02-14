# Use an official Python runtime as a parent image
FROM python:3.10

ENV PYTHONDONTWRITEBYTECODE=1

EXPOSE 80

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY app.py .

CMD ["python", "app.py"]
