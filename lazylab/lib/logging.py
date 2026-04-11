import logging


class LazyLabLogFormatter(logging.Formatter):
    def __init__(self) -> None:
        super().__init__(fmt="%(asctime)s [%(levelname)s] %(name)s: %(message)s", datefmt="%Y-%m-%d %H:%M:%S")


ll = logging.getLogger("lazylab")
ll.setLevel(logging.DEBUG)
