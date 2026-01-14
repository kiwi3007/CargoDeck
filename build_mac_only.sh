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
echo "[3/3] Packaging .app (with Icon & Launcher)..."
APP_NAME="build_artifacts/Playerr"
rm -rf "${APP_NAME}.app"
mkdir -p "${APP_NAME}.app/Contents/MacOS"
mkdir -p "${APP_NAME}.app/Contents/Resources"

# Copy Executable & Core Files
cp -a build_artifacts/osx-arm64/* "${APP_NAME}.app/Contents/MacOS/"

# Copy UI Assets
mkdir -p "${APP_NAME}.app/Contents/MacOS/_output/UI"
cp -a _output/UI/* "${APP_NAME}.app/Contents/MacOS/_output/UI/" 2>/dev/null || true

# --- ICON GENERATION ---
ICON_SRC="frontend/src/assets/app_logo.png"
if [ -f "$ICON_SRC" ]; then
    echo "Generating .icns icon..."
    ICONSET_DIR="MyIcon.iconset"
    mkdir -p "$ICONSET_DIR"
    
    sips -z 16 16     "$ICON_SRC" --out "$ICONSET_DIR/icon_16x16.png" > /dev/null
    sips -z 32 32     "$ICON_SRC" --out "$ICONSET_DIR/icon_16x16@2x.png" > /dev/null
    sips -z 32 32     "$ICON_SRC" --out "$ICONSET_DIR/icon_32x32.png" > /dev/null
    sips -z 64 64     "$ICON_SRC" --out "$ICONSET_DIR/icon_32x32@2x.png" > /dev/null
    sips -z 128 128   "$ICON_SRC" --out "$ICONSET_DIR/icon_128x128.png" > /dev/null
    sips -z 256 256   "$ICON_SRC" --out "$ICONSET_DIR/icon_128x128@2x.png" > /dev/null
    sips -z 256 256   "$ICON_SRC" --out "$ICONSET_DIR/icon_256x256.png" > /dev/null
    sips -z 512 512   "$ICON_SRC" --out "$ICONSET_DIR/icon_256x256@2x.png" > /dev/null
    sips -z 512 512   "$ICON_SRC" --out "$ICONSET_DIR/icon_512x512.png" > /dev/null
    sips -z 1024 1024 "$ICON_SRC" --out "$ICONSET_DIR/icon_512x512@2x.png" > /dev/null
    
    iconutil -c icns "$ICONSET_DIR" -o "${APP_NAME}.app/Contents/Resources/AppIcon.icns"
    rm -rf "$ICONSET_DIR"
else
    echo "Warning: Icon source not found ($ICON_SRC)"
fi

# --- LAUNCHER SCRIPT ---
# Creates a launcher that sets the working directory to the MacOS folder
cat > "${APP_NAME}.app/Contents/MacOS/Launcher" <<EOF
#!/bin/bash
DIR="\$( cd "\$( dirname "\${BASH_SOURCE[0]}" )" && pwd )"
cd "\$DIR"
./Playerr.Host
EOF

chmod +x "${APP_NAME}.app/Contents/MacOS/Launcher"
chmod +x "${APP_NAME}.app/Contents/MacOS/Playerr.Host"

# Create Info.plist (Pointing to Launcher)
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
    <string>0.4.0</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleExecutable</key>
    <string>Launcher</string>
    <key>CFBundleIconFile</key>
    <string>AppIcon</string>
    <key>NSHighResolutionCapable</key>
    <true/>
    <key>LSUIElement</key>
    <false/>
</dict>
</plist>

echo "=================================="
echo "Fast Build Complete!"
echo "Run with: open ${APP_NAME}.app"
echo "=================================="
