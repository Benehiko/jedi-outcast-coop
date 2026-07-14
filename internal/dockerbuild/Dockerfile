# Build image for the JK2-SP co-op engine. Carries the native Linux toolchain
# and libraries plus the mingw-w64 cross toolchain, so the same image builds
# both the Linux (ELF) and Windows (PE) engine. The image is built inside the
# vee "docker" template VM via the Docker Engine API; the host installs nothing.
#
# The apt set mirrors internal/prereq (AptPackages) for the Linux target and
# adds mingw-w64 for the Windows cross-build. Keep it in lockstep with
# prereq.AptPackages and internal/dockerbuild/target.go (mingwPrefix).
FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      cmake \
      ninja-build \
      build-essential \
      libsdl2-dev \
      libopenal-dev \
      zlib1g-dev \
      libpng-dev \
      libjpeg-dev \
      mingw-w64 \
      mingw-w64-tools \
 && rm -rf /var/lib/apt/lists/*

# The source is bind-mounted at /src (a virtiofs share from the host); the build
# writes to /src/<build subdir>, which the host sees through the share. The
# actual configure/build command is supplied at `docker run` time.
WORKDIR /src
