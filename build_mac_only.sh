#!/bin/bash
set -e

echo "=================================="
echo "Playerr FAST Build (macOS Only)"
echo "=================================="

# 1. Build Frontend (Only if requested, or we can skip for pure backend dev?)
# For now, let's keep it but maybe we can add a flag to skip.
# Assuming user wants frontend updates too since we are debugging UI.
echo "[1/3] Building Frontend..."
npm run build

# 2. Publish Backend for macOS (ARM64 default for this user)
echo "[2/3] Publishing Backend (OSX-ARM64)..."
dotnet publish src/Playerr.Host/Playerr.Host.csproj \
    -c Release \
    -r osx-arm64 \
    --self-contained true \
    -p:PublishSingleFile=false \
    -o build_artifacts/osx-arm64

# 3. Create .app Bundle
echo "[3/3] Packaging .app..."
APP_NAME="build_artifacts/Playerr"
rm -rf "${APP_NAME}.app"
mkdir -p "${APP_NAME}.app/Contents/MacOS"
mkdir -p "${APP_NAME}.app/Contents/Resources"

# Copy Executable & Core Files
cp -a build_artifacts/osx-arm64/* "${APP_NAME}.app/Contents/MacOS/"

# Copy UI Assets
mkdir -p "${APP_NAME}.app/Contents/MacOS/_output/UI"
cp -a _output/UI/* "${APP_NAME}.app/Contents/MacOS/_output/UI/" 2>/dev/null || true
# Copy also to Resources just in case of path resolution issues
# Copy also to Resources just in case of path resolution issues (optional but kept for safety)
# cp -a _output/UI/* "${APP_NAME}.app/Contents/Resources/" 2>/dev/null || true

# Create Info.plist
cat > "${APP_NAME}.app/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleName</key>
    <string>Playerr</string>
    <key>CFBundleDisplayName</key>
    <string>Playerr</string>
    <key>CFBundleIdentifier</key>
    <string>com.playerr.app</string>
    <key>CFBundleVersion</key>
    <string>0.1.0-beta</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleExecutable</key>
    <string>Playerr.Host</string>
    <key>CFBundleIconFile</key>
    <string>AppIcon</string>
    <key>NSHighResolutionCapable</key>
    <true/>
</dict>
</plist>
EOF

chmod +x "${APP_NAME}.app/Contents/MacOS/Playerr.Host"

echo "=================================="
echo "Fast Build Complete!"
echo "Run with: open ${APP_NAME}.app"
echo "=================================="
