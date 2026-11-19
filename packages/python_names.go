package packages

// pyStdlib is sys.stdlib_module_names for Python 3.8 to 3.14, top-level
// names only, including modules removed along the way. Private _names are
// skipped separately.
var pyStdlib = setOf(`
abc aifc annotationlib antigravity argparse array ast asynchat asyncio asyncore atexit audioop base64 bdb
binascii bisect builtins bz2 cProfile calendar cgi cgitb chunk cmath cmd code codecs codeop collections
colorsys compileall compression concurrent configparser contextlib contextvars copy copyreg crypt csv ctypes
curses dataclasses datetime dbm decimal difflib dis distutils doctest email encodings ensurepip enum errno
faulthandler fcntl filecmp fileinput fnmatch fractions ftplib functools gc genericpath getopt getpass gettext
glob graphlib grp gzip hashlib heapq hmac html http idlelib imaplib imghdr imp importlib inspect io ipaddress
itertools json keyword lib2to3 linecache locale logging lzma mailbox mailcap marshal math mimetypes mmap
modulefinder msilib msvcrt multiprocessing netrc nis nntplib nt ntpath nturl2path numbers opcode operator
optparse os ossaudiodev pathlib pdb pickle pickletools pipes pkgutil platform plistlib poplib posix posixpath
pprint profile pstats pty pwd py_compile pyclbr pydoc pydoc_data pyexpat queue quopri random re readline
reprlib resource rlcompleter runpy sched secrets select selectors shelve shlex shutil signal site smtpd
smtplib sndhdr socket socketserver spwd sqlite3 sre_compile sre_constants sre_parse ssl stat statistics
string stringprep struct subprocess sunau symtable sys sysconfig syslog tabnanny tarfile telnetlib tempfile
termios textwrap this threading time timeit tkinter token tokenize tomllib trace traceback tracemalloc tty
turtle turtledemo types typing unicodedata unittest urllib uu uuid venv warnings wave weakref webbrowser
winreg winsound wsgiref xdrlib xml xmlrpc zipapp zipfile zipimport zlib zoneinfo
`)

// pyHost lists modules provided by the program that runs the script
// (MicroPython, CircuitPython, Blender, Kodi, IDA, editors and 3D tools,
// AWS Glue, Jython, Pyodide) rather than by a PyPI package.
var pyHost = setOf(`
machine network micropython utime uos ujson urequests usocket ubinascii ucollections uasyncio ustruct
uselect ussl uhashlib urandom uio esp esp32 pyb rp2 bluetooth framebuf
board busio digitalio analogio pwmio microcontroller supervisor displayio
bpy bmesh mathutils bgl blf gpu gpu_extras aud bpy_extras freestyle idprop imbuf
xbmc xbmcgui xbmcaddon xbmcplugin xbmcvfs
idaapi idc idautils
sublime sublime_plugin gimpfu qgis maya hou nuke unreal adsk renpy awsglue
java javax js pyodide_js
`)

var pyHostPrefixes = []string{"adafruit_", "ida_"}

// pyImportDist maps import names to the differently named PyPI distribution
// that provides them. Keys are lower case.
var pyImportDist = map[string]string{
	"airflow": "apache-airflow", "apt": "python-apt", "apt_pkg": "python-apt", "argon2": "argon2-cffi",
	"attr": "attrs", "azure": "azure-core", "barcode": "python-barcode", "binance": "python-binance",
	"bio": "biopython", "brownie": "eth-brownie", "bs4": "beautifulsoup4", "cairo": "pycairo",
	"clr": "pythonnet", "corsheaders": "django-cors-headers", "crispy_forms": "django-crispy-forms",
	"crypto": "pycryptodome", "cryptodome": "pycryptodomex", "cups": "pycups", "cv2": "opencv-python",
	"dateutil": "python-dateutil", "dbus": "dbus-python", "debug_toolbar": "django-debug-toolbar",
	"decouple": "python-decouple", "discord": "discord-py", "django_filters": "django-filter",
	"dns": "dnspython", "docx": "python-docx", "dotenv": "python-dotenv", "environ": "django-environ",
	"faiss": "faiss-cpu", "fitz": "pymupdf", "gi": "pygobject", "git": "gitpython", "github": "pygithub",
	"gitlab": "python-gitlab", "google": "protobuf", "googleapiclient": "google-api-python-client",
	"grpc": "grpcio", "jenkins": "python-jenkins", "jose": "python-jose", "jwt": "pyjwt",
	"kafka": "kafka-python", "ldap": "python-ldap", "libvirt": "libvirt-python", "magic": "python-magic",
	"mpl_toolkits": "matplotlib", "multipart": "python-multipart", "mysqldb": "mysqlclient",
	"nacl": "pynacl", "nmap": "python-nmap", "opengl": "pyopengl", "openssl": "pyopenssl", "osgeo": "gdal",
	"paho": "paho-mqtt", "pil": "pillow", "pkg_resources": "setuptools", "pptx": "python-pptx",
	"pythoncom": "pywin32", "pywintypes": "pywin32", "rest_framework": "djangorestframework",
	"rpi": "rpi-gpio", "ruamel": "ruamel-yaml", "serial": "pyserial", "skimage": "scikit-image",
	"sklearn": "scikit-learn", "slack": "slackclient", "slugify": "python-slugify", "smbus": "smbus2",
	"snowflake": "snowflake-connector-python", "socks": "pysocks", "speech_recognition": "speechrecognition",
	"st_aggrid": "streamlit-aggrid", "storages": "django-storages", "talib": "ta-lib",
	"telebot": "pytelegrambotapi", "telegram": "python-telegram-bot", "usb": "pyusb",
	"vertexai": "google-cloud-aiplatform", "vlc": "python-vlc", "weaviate": "weaviate-client",
	"win32api": "pywin32", "win32con": "pywin32", "win32gui": "pywin32", "wx": "wxpython",
	"xlib": "python-xlib", "yaml": "pyyaml", "zmq": "pyzmq",
}
