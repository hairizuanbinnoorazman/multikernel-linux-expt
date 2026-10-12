FROM python:3.13-slim
WORKDIR /app
COPY requirements.txt .
RUN python -m pip install --no-cache-dir --index-url https://pypi.org/simple --report /app/pip-report.json -r requirements.txt
COPY app.py .
CMD ["python", "app.py"]
