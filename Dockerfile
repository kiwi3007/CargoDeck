# Stage 1: Build the Frontend (React)
FROM node:18 AS frontend
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm install
COPY frontend/ ./frontend/
COPY tsconfig.json ./
COPY frontend/build/webpack.config.js ./frontend/build/
RUN npm run build

# Stage 2: Build the Backend (.NET)
FROM mcr.microsoft.com/dotnet/sdk:8.0 AS backend
WORKDIR /src

# Copy solution and build configuration files
COPY src/Playerr.sln ./
COPY src/Directory.Build.props ./
COPY src/Directory.Build.targets ./
COPY src/NuGet.config ./

# Copy each project file explicitly (best for layer caching and path integrity)
COPY src/Playerr.Api.V3/*.csproj Playerr.Api.V3/
COPY src/Playerr.Common/*.csproj Playerr.Common/
COPY src/Playerr.Console/*.csproj Playerr.Console/
COPY src/Playerr.Core/*.csproj Playerr.Core/
COPY src/Playerr.Host/*.csproj Playerr.Host/
COPY src/Playerr.Http/*.csproj Playerr.Http/
COPY src/Playerr.SignalR/*.csproj Playerr.SignalR/
COPY src/Playerr.UsbHelper/*.csproj Playerr.UsbHelper/

# Restore dependencies
RUN dotnet restore Playerr.sln

# Copy everything else and build
COPY src/ ./
COPY frontend/src/assets/app_logo.ico ../frontend/src/assets/
RUN dotnet publish Playerr.Host/Playerr.Host.csproj -c Release -o /app/publish

# Stage 3: Final Runtime Image
FROM mcr.microsoft.com/dotnet/aspnet:8.0
WORKDIR /app

# Install runtime dependencies for Switch USB support (Python + libusb)
RUN apt-get update && apt-get install -y \
    python3 \
    python3-pip \
    libusb-1.0-0 \
    && rm -rf /var/lib/apt/lists/*

# Install pyusb
RUN pip3 install --break-system-packages pyusb

COPY --from=backend /app/publish .

# Ensure no personal configs are included in the image
RUN rm -f /app/config/*.json && rm -f /app/settings/*.json && rm -f /app/appsettings.Development.json

# Copy frontend artifacts to where the backend expects them
COPY --from=frontend /src/_output/UI ./_output/UI

# Create config and media directories
RUN mkdir -p /app/config /media

# Expose port 2727
EXPOSE 2727
ENV ASPNETCORE_URLS=http://+:2727
ENV DOTNET_RUNNING_IN_CONTAINER=true

ENTRYPOINT ["dotnet", "Playerr.Host.dll"]
