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

# Copy csproj and restore as distinct layers
COPY src/*/*.csproj ./
RUN for file in $(ls *.csproj); do mkdir -p ${file%.*}/ && mv $file ${file%.*}/; done
COPY src/*.sln ./
COPY src/Directory.Build.props ./
COPY src/Directory.Build.targets ./
COPY src/NuGet.config ./
RUN dotnet restore

# Copy everything else and build
COPY src/ ./src/
# Copy the icon file so that the relative path in .csproj (../../frontend/...) resolves correctly
COPY frontend/src/assets/app_logo.ico ./frontend/src/assets/
RUN dotnet publish src/Playerr.Host/Playerr.Host.csproj -c Release -o /app/publish

# Stage 3: Final Runtime Image
FROM mcr.microsoft.com/dotnet/aspnet:8.0
WORKDIR /app
COPY --from=backend /app/publish .

# Ensure no personal configs are included in the image
RUN rm -f /app/config/*.json && rm -f /app/settings/*.json && rm -f /app/appsettings.Development.json
# Copy frontend artifacts to where the backend expects them
# Ensure this matches the static file path in Program.cs (usually _output or similar)
COPY --from=frontend /src/_output/UI ./_output/UI

# Create config and media directories
RUN mkdir -p /app/config /media

# Expose port 2727
EXPOSE 2727
ENV ASPNETCORE_URLS=http://+:2727
ENV DOTNET_RUNNING_IN_CONTAINER=true

ENTRYPOINT ["dotnet", "Playerr.Host.dll"]
