## Releasing

These are the general steps to release a new package version:

1. Run the following command `just pre-release`;
2. Open a pull request with the changes;
3. Once the pull request is merged the Continuous Delivery workflow will build the release version, create the tag and create a draft [GitHub release](https://docs.github.com/en/repositories/releasing-projects-on-github/about-releases) with the website and change notes;
4. Review the draft GitHub release and, if everything is okay, release it;
