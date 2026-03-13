Build and push the Playerr Docker image to GHCR.

Run the following steps:
1. Check for uncommitted changes with `git status`. If the working tree is dirty, create a commit for all staged and unstaged changes before proceeding. Follow the standard commit process (git status, git diff, git log, stage relevant files, commit with Co-Authored-By trailer).
2. Compute the version from git: `VERSION=$(git describe --dirty --always --abbrev=7)`
3. Build the Docker image: `docker build -f Dockerfile.gobackend --build-arg VERSION="$VERSION" -t ghcr.io/kiwi3007/playerr:refactor .`
4. Push to GHCR: `docker push ghcr.io/kiwi3007/playerr:refactor`
5. Report the version used and success or any errors.
