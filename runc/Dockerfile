# Use a base image with necessary tools
FROM ubuntu:20.04

# Install dependencies
RUN apt-get update && apt-get install -y \
    build-essential \
    libseccomp-dev \
    git \
    wget

# Clone and build runc from source
RUN git clone https://github.com/opencontainers/runc.git /usr/src/runc
WORKDIR /usr/src/runc
RUN make && make install

# Set the working directory to /runc to run commands from
WORKDIR /runc

# Set the entrypoint to runc
ENTRYPOINT ["runc"]