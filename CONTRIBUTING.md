## Contributing

Hey there!!! We are happy that you would like to contribute to make this project great!

On the section bellow you find all the necessary steps to start contributing.

All contributions to this project are released under the project [The MIT License](https://opensource.org/license/mit) license.

This project is released with a Contributor [Code of Conduct](CODE_OF_CONDUCT.md). By participating in this project you agree to abide by its terms.

Happy coding :-).

### Submitting a pull request

These are the general steps to open a pull request with your contribution to the project:

1. [Fork](https://github.com/josimar-silva/smaug/fork) and clone the repository;

```sh
git clone https://github.com/josimar-silva/smaug && cd smaug
# OR
git clone git@github.com:josimar-silva/smaug && cd ksmaugn
```

2. Build the project. Make sure you have [Go](https://go.dev/) `1.26+` or later installed;

```sh
just build
```

3. Create a new branch: `git checkout -b your-branch-name`;
4. Start committing your changes. Follow the [conventional commit specification](https://www.conventionalcommits.org/) while doing so;
4. Add your contribution and appropriate tests. Make sure all tests are green;

```sh
just test
```

5. Push to your fork and [submit a pull request](https://github.com/josimar-silva/smaug/compare);
6. Enjoying a cup of coffee while our team review your pull request.

Here are a few things to keep in mind while preparing your pull request:

- Write tests for your changes; 
- Keep your changes focused. If there are independent changes, consider if they could be submitted as separate pull requests;
- Write good [commit messages](https://github.blog/2022-06-30-write-better-commits-build-better-projects/).

With that in mind, the chances of your pull request be accepted will be quite high.

-------
## Resources
- [How to contribute to Open Source?](https://opensource.guide/how-to-contribute/);
- [Write better commits, build better projects](https://github.blog/2022-06-30-write-better-commits-build-better-projects/);
- [About Pull Requests](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/proposing-changes-to-your-work-with-pull-requests/about-pull-requests);
