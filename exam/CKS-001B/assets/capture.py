"""Bounded, read-only dispatch for all sixteen tasks."""
import json
import sys

from capture_files import capture as files
from workload_capture import capture as workloads


def capture(question):
    if question == "q1":
        from falco_capture import capture as falco
        return falco()
    if question in ("q2", "q4", "q13"):
        from network_capture import capture as network
        result = network(question)
        if question == "q2":
            result["allowed"] = result["allowed"] and result["scope"]
        elif question == "q13":
            result["denied"] = result["denied"] and result["scope"]
        return result
    if question in ("q3", "q6", "q8", "q10", "q15"):
        return workloads(question)
    if question == "q5":
        from rbac_capture import capture as rbac
        return rbac()
    if question == "q7":
        result = files(question)
        # The starting Ubuntu fixture already copies just the binary.
        for key in ("minimal", "binary", "nonroot", "exec"):
            result[key] = result[key] and result["base"]
        return result
    if question == "q9":
        from psa_capture import capture as psa
        return psa()
    if question == "q11":
        from sbom_capture import capture as sbom
        return sbom()
    if question == "q12":
        from ingress_capture import capture as ingress
        result = ingress()
        result["configuration"] = result["class"] and result["annotations"]
        result["traffic"] = result["redirect"] and result["https"]
        return result
    if question == "q14":
        from mesh_capture import capture as mesh
        return mesh()
    if question == "q16":
        from etcd_capture import capture as etcd
        return etcd()
    raise ValueError("unsupported question")


if __name__ == "__main__":
    if len(sys.argv) != 2:
        raise SystemExit("usage: capture.py q<number>")
    try:
        print(json.dumps(capture(sys.argv[1])))
    except Exception as error:
        # Supporting failures must never look like a missing candidate object.
        print(("capture infrastructure error: " + str(error)).replace("NotFound", "missing supporting object"),
              file=sys.stderr)
        sys.exit(1)
