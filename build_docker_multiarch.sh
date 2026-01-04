#!/bin/bash
# Script para construir imágenes Docker multi-arquitectura para Playerr

echo "===================================================="
echo "      Playerr Multi-Arch Docker Build Tool          "
echo "===================================================="

# Nombre de la imagen
IMAGE_NAME="playerr:latest"

# 1. Verificar si Buildx está disponible
if ! docker buildx version >/dev/null 2>&1; then
    echo "Error: Docker Buildx no está instalado o habilitado."
    exit 1
fi

# 2. Crear un nuevo constructor si no existe uno compatible
BUILDER_NAME="playerr-builder"
if ! docker buildx inspect "$BUILDER_NAME" >/dev/null 2>&1; then
    echo "Creando nuevo constructor multi-plataforma..."
    docker buildx create --name "$BUILDER_NAME" --use
    docker buildx inspect --bootstrap
else
    docker buildx use "$BUILDER_NAME"
fi

# 3. Ejecutar el build multi-arquitectura
echo "Iniciando build para linux/amd64 y linux/arm64..."
echo "Esto puede tardar unos minutos ya que compila para ambas plataformas."

# Nota: Usamos --load solo si es una sola plataforma. 
# Para multi-plataforma local, Docker no permite --load directamente a la biblioteca local fácilmente.
# Por lo tanto, lo construimos y lo dejamos en el cache o lo exportamos.
docker buildx build \
    --platform linux/amd64,linux/arm64 \
    -t "$IMAGE_NAME" \
    --output type=docker \
    . || {
    echo "Nota: El build multi-plataforma directo a la biblioteca local de Docker tiene limitaciones."
    echo "Intentando construir solo para la arquitectura actual para uso inmediato..."
    docker build -t "$IMAGE_NAME" .
}

echo "===================================================="
echo "          Build finalizado correctamente            "
echo "===================================================="
