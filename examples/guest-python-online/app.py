"""Small online app: fetch public package metadata with verified HTTPS."""
import json
import platform
import requests

response = requests.get("https://pypi.org/pypi/requests/json", timeout=30, verify=True)
response.raise_for_status()
assert response.json()["info"]["name"] == "requests"
print(json.dumps({"app": "multikernel-python-online", "python": platform.python_version(),
                  "requests": requests.__version__, "https_status": response.status_code,
                  "source": response.url, "tls_verification": True}, sort_keys=True))
