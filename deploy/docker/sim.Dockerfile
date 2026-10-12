# Battery-site simulator image (pymodbus SunSpec inverter + meter).
FROM python:3.13-slim
RUN pip install --no-cache-dir pymodbus==3.15.0
WORKDIR /app
# manifest.py resolves contracts/sunspec-profile.json at parents[2],
# so the repo layout must be preserved under /app.
COPY contracts/sunspec-profile.json contracts/sunspec-profile.json
COPY sim/voltix_sim sim/voltix_sim
ENV PYTHONPATH=/app/sim
RUN useradd -r -u 10002 -m voltsim && mkdir -p /app && chown -R voltsim /app
USER voltsim
EXPOSE 1502 1503 1504
CMD ["python", "-m", "voltix_sim", "--host", "0.0.0.0"]
