SPK_NAME = playerr
SPK_VERS = 0.1.2
SPK_REV = 1
SPK_ICON = src/app_logo.png

DEPENDS = cross/dotnet-runtime

MAINTAINER = Maikboarder
DESCRIPTION = Playerr is a self-hosted game library manager and PVR.
DISPLAY_NAME = Playerr
CHANGELOG = "Initial Release"

HOMEPAGE = https://playerr.app
LICENSE  = MIT

# Service setup
SERVICE_USER = auto
SERVICE_SETUP = src/service-setup.sh

# Port configuration
SERVICE_PORT = 2727
SERVICE_PORT_TITLE = Playerr Web UI

# Data configuration (creates /var/packages/playerr/var/config)
# We want to map this to standard location
USE_ALTERNATE_TMPDIR = 1

include ../../mk/spksrc.spk.mk
