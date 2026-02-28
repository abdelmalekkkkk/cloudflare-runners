#!/bin/bash

./config.sh --disableupdate --no-default-labels --ephemeral --unattended --name $CF_RUNNER_NAME --url $CF_RUNNER_REPO_URL --token $CF_RUNNER_TOKEN --labels $CF_RUNNER_LABELS
./run.sh
