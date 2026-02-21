FROM --platform=linux/amd64 ubuntu:24.04

RUN apt update -y

RUN apt install curl -y

RUN useradd -ms /bin/bash runner

USER runner

WORKDIR /runner

RUN \
    curl -o actions-runner-linux-x64-2.331.0.tar.gz -L https://github.com/actions/runner/releases/download/v2.331.0/actions-runner-linux-x64-2.331.0.tar.gz\
    && echo "5fcc01bd546ba5c3f1291c2803658ebd3cedb3836489eda3be357d41bfcf28a7  actions-runner-linux-x64-2.331.0.tar.gz" | sha256sum -c\
    && tar xzf ./actions-runner-linux-x64-2.331.0.tar.gz\
    && rm -f actions-runner-linux-x64-2.331.0.tar.gz

USER root

RUN ./bin/installdependencies.sh

USER runner
