"""Python setup.py for project_name package"""
import io
import os
from setuptools import find_packages, setup


def read(*paths, **kwargs):
    content = ""
    with io.open(
        os.path.join(os.path.dirname(__file__), *paths),
        encoding=kwargs.get("encoding", "utf8"),
    ) as open_file:
        content = open_file.read().strip()
    return content


def read_requirements(path):
    return [
        line.strip()
        for line in read(path).split("\n")
        if not line.startswith(('"', "#", "-", "git+"))
    ]


setup(
    name="simple_uploader",
    version=read("simple_uploader", "VERSION"),
    description="Simple uploader python client",
    url="https://github.com/louis-she/simple-uploader",
    long_description=read("../../README.md"),
    long_description_content_type="text/markdown",
    author="Chenglu",
    packages=find_packages(),
    install_requires=read_requirements("requirements.txt"),
)