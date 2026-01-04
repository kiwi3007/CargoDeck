#!/bin/bash
set -e

echo "=================================="
echo "Playerr Build (macOS Intel x64)"
echo "=================================="

# 1. Build Frontend
echo "[1/3] Building Frontend..."
npm run build

# 2. Publish Backend for macOS Intel
echo "[2/3] Publishing Backend (OSX-X64)..."
dotnet publish src/Playerr.Host/Playerr.Host.csproj \
    -c Release \
    -r osx-x64 \
    --self-contained true \
    -p:PublishSingleFile=false \
    -o build_artifacts/osx-x64

# 3. Create .app Bundle
echo "[3/3] Packaging .app..."
APP_NAME="build_artifacts/Playerr-Intel"
rm -rf "${APP_NAME}.app"
mkdir -p "${APP_NAME}.app/Contents/MacOS"
mkdir -p "${APP_NAME}.app/Contents/Resources"

# Copy Executable & Core Files
cp -a build_artifacts/osx-x64/* "${APP_NAME}.app/Contents/MacOS/"

# Copy UI Assets
mkdir -p "${APP_NAME}.app/Contents/MacOS/_output/UI"
cp -a _output/UI/* "${APP_NAME}.app/Contents/MacOS/_output/UI/" 2>/dev/null || true

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
    <string>app.playerr.desktop</string>
    <key>CFBundleVersion</key>
    <string>0.1.0</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleSignature</key>
    <string>????</string>
    <key>CFBundleExecutable</key>
    <string>Playerr.Host</string>
    <key>LSMinimumSystemVersion</key>
    <string>10.15</string>
</dict>
</plist>
EOF

# Create DMG (if hdiutil is available)
if command -v hdiutil >/dev/null; then
    echo "Creating .dmg for Intel..."
    rm -f "build_artifacts/Playerr-Intel.dmg"
    hdiutil create -volname "Playerr Intel" -srcfolder "${APP_NAME}.app" -ov -format UDZO "build_artifacts/Playerr-Intel.dmg"
fi

echo "=================================="
echo "Intel Build Complete!"
echo "Artifact: build_artifacts/Playerr-Intel.dmg"
echo "=================================="
