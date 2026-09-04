# Release Process

This document describes the release process for Gateway Lens.

### Pre-requisites

You must have the proper permissions to run workflows and push tags.

### Steps

1. Run the [Update Version](https://github.com/nginxinc/gateway-lens/actions/workflows/update-chart-version.yml) workflow with the new release version. This creates a pull request that updates the version strings in the repository.
2. Merge this pull request in.
3. With a local checkout of the repository, ensure you are on the latest commit, then create and push the release tag. For example:

    ```shell
    git clone --origin upstream git@github.com:nginxinc/gateway-lens.git
    cd gateway-lens/
    ```

    ```shell
    git pull --ff-only
    git tag v0.1.0            # replace with the proper release version
    git push upstream v0.1.0  # replace with the proper release version
    ```

This automatically triggers the release pipeline to run, which will create the [release](https://github.com/nginxinc/gateway-lens/releases) and publish the artifacts.
