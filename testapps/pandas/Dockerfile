FROM python:3.10

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
RUN mkdir -p /usr/local/share/tcdata && python -c "import numpy as np, pandas as pd; rng = np.random.default_rng(0); n = 250_000; pd.DataFrame({\"key\": rng.integers(0, 1000, n), \"a\": rng.standard_normal(n), \"b\": rng.standard_normal(n), \"c\": rng.integers(0, 10**6, n)}).to_csv(\"/usr/local/share/tcdata/pandas-data.csv\", index=False)"
COPY app.py .

CMD ["python", "app.py"]
