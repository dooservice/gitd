# The agent's sandbox: OpenHands and git, nothing else. No socket, no credential, no
# network is needed for git: the workspace gitd made is bound at /workspace.
FROM python:3.13-slim
RUN apt-get update && apt-get install -y --no-install-recommends git && rm -rf /var/lib/apt/lists/*
ARG OPENHANDS_VERSION=1.55.1
RUN pip install --no-cache-dir "openhands-sdk==${OPENHANDS_VERSION}" "openhands-tools==${OPENHANDS_VERSION}"
COPY agent.py /agent/agent.py
WORKDIR /workspace
ENTRYPOINT ["python", "/agent/agent.py"]
