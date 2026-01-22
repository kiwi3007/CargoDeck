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
WORKDIR /app

# Copy solution and build configuration files (preserving directory structure)
COPY src/*.sln src/
COPY src/Directory.Build.props src/
COPY src/Directory.Build.targets src/
COPY src/NuGet.config src/

# Copy each project file explicitly (best for layer caching and path integrity)
COPY src/Playerr.Api.V3/*.csproj src/Playerr.Api.V3/
COPY src/Playerr.Common/*.csproj src/Playerr.Common/
COPY src/Playerr.Console/*.csproj src/Playerr.Console/
COPY src/Playerr.Core/*.csproj src/Playerr.Core/
COPY src/Playerr.Host/*.csproj src/Playerr.Host/
COPY src/Playerr.Http/*.csproj src/Playerr.Http/
COPY src/Playerr.SignalR/*.csproj src/Playerr.SignalR/
COPY src/Playerr.UsbHelper/*.csproj src/Playerr.UsbHelper/

# Restore dependencies
RUN dotnet restore src/Playerr.sln

# Copy everything else and build
COPY src/ src/
COPY frontend/src/assets/app_logo.ico frontend/src/assets/
RUN dotnet publish src/Playerr.Host/Playerr.Host.csproj -c Release -o /app/publish

# Stage 3: Final Runtime Image
FROM mcr.microsoft.com/dotnet/aspnet:8.0
WORKDIR /app
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
