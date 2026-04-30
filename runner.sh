#!/usr/bin/env bash

if [[ ! -d /data ]]; then
  mkdir -p /data
fi

cd /data

CONFIG_ARG=""
EXTRA_ARGS=""
RUN_ARGS=""

DIND_STARTED=0
if [[ $UID -eq 0 ]]; then
  DIND_STARTED=1
  # Start docker in background
  (dockerd --host='unix:///var/run/docker.sock' ${DOCKERD_ARGS})&
  trap 'killall -9 dockerd' EXIT

  # wait to docker start
  try=0
  while ! docker ps; do
    try=$((try + 1))
    sleep 1s
    if [[ $try -eq ${GITEA_MAX_DOCKER_ATTEMPTS:-20} ]]; then
      echo docker not ready
      exit 1
    fi
  done
else
  RUN_ARGS="${RUN_ARGS} --host-workdir-parent=/workspace"
fi

RUNNER_STATE_FILE=${RUNNER_STATE_FILE:-'.runner'}
rm -f $RUNNER_STATE_FILE

if [[ ! -z "${CONFIG_FILE}" ]]; then
  CONFIG_ARG="--config ${CONFIG_FILE}"
fi
if [[ ! -z "${GITEA_RUNNER_LABELS}" ]]; then
  EXTRA_ARGS="${EXTRA_ARGS} --labels ${GITEA_RUNNER_LABELS}"
fi
if [[ ! -z "${GITEA_RUNNER_EXTERNAL_CACHE}" ]]; then
  if [[ -z "${GITEA_RUNNER_EXTERNAL_CACHE_TOKEN}" ]]; then
    echo "External cache require TOKEN"
    exit 2
  fi
  RUN_ARGS="${RUN_ARGS} --cache-external-cache ${GITEA_RUNNER_EXTERNAL_CACHE} --cache-external-secret ${GITEA_RUNNER_EXTERNAL_CACHE_TOKEN}"
  unset GITEA_RUNNER_EXTERNAL_CACHE_TOKEN
fi
([[ $DIND_STARTED -eq 0 ]] && [[ -z "${GITEA_RUNNER_LABELS}" ]]) && echo "Recomends add GITEA_RUNNER_LABELS to run in host because cannot start dockerd"

# In case no token is set, it's possible to read the token from a file, i.e. a Docker Secret
if [[ -z "${GITEA_RUNNER_REGISTRATION_TOKEN}" ]] && [[ -f "${GITEA_RUNNER_REGISTRATION_TOKEN_FILE}" ]]; then
  GITEA_RUNNER_REGISTRATION_TOKEN=$(cat "${GITEA_RUNNER_REGISTRATION_TOKEN_FILE}")
fi

if [[ "${GITEA_RUNNER_EPHEMERAL}" -eq 1 ]]; then
  EXTRA_ARGS="${EXTRA_ARGS} --ephemeral"
fi
if [[ "${GITEA_RUNNER_ONCE}" -eq 1 ]]; then
  RUN_ARGS="${RUN_ARGS} --once"
fi

# Use the same ENV variable names as https://github.com/vegardit/docker-gitea-act-runner
test -f "$RUNNER_STATE_FILE" || echo "$RUNNER_STATE_FILE is missing or not a regular file"

try=0
if [[ ! -s "$RUNNER_STATE_FILE" ]]; then
  try=$((try + 1))
  success=0

  # The point of this loop is to make it simple, when running both gitea-runner and gitea in docker,
  # for the gitea-runner to wait a moment for gitea to become available before erroring out.  Within
  # the context of a single docker-compose, something similar could be done via healthchecks, but
  # this is more flexible.
  while [[ $success -eq 0 ]] && [[ $try -lt ${GITEA_MAX_REG_ATTEMPTS:-3} ]]; do
    gitea-runner register \
      --instance "${GITEA_INSTANCE_URL}" \
      --token    "${GITEA_RUNNER_REGISTRATION_TOKEN}" \
      --name     "${GITEA_RUNNER_NAME:-`hostname`}" \
      ${CONFIG_ARG} ${EXTRA_ARGS} --no-interactive 2>&1 | tee /tmp/reg.log

    cat /tmp/reg.log | grep 'Runner registered successfully' > /dev/null
    if [[ $? -eq 0 ]]; then
      echo "SUCCESS"
      success=1
    else
      echo "Waiting to retry ..."
      sleep 5
    fi
    rm /tmp/reg.log
  done
fi
# Prevent reading the token from the gitea-runner process
unset GITEA_RUNNER_REGISTRATION_TOKEN
unset GITEA_RUNNER_REGISTRATION_TOKEN_FILE

exec gitea-runner daemon ${CONFIG_ARG} ${RUN_ARGS}
