#!/usr/bin/env bash
set -e

ROS_DISTRO="humble"
INSTALL_PREFIX="/opt/ros/${ROS_DISTRO}"

BASE_DIR="$(realpath "$(dirname "$0")")/custom_interfaces"

if [[ $EUID -ne 0 ]]; then
    echo "Please run as root: sudo $0"
    exit 1
fi

if ! source "${INSTALL_PREFIX}/setup.bash" 2>/dev/null; then
    echo "ROS 2 ${ROS_DISTRO} not found at ${INSTALL_PREFIX}"
    exit 1
fi

echo "Searching for ROS 2 packages in ${BASE_DIR}..."
mapfile -t PKG_PATHS < <(find "${BASE_DIR}" -name "package.xml" -exec dirname {} \;)
PKG_NAMES=()

for pkg_path in "${PKG_PATHS[@]}"; do
    pkg_name=$(grep -m1 "<name>" "${pkg_path}/package.xml" | sed -E 's/.*<name>([^<]+)<\/name>.*/\1/')
    if [[ -n "${pkg_name}" ]]; then
        PKG_NAMES+=("${pkg_name}")
    fi
done

if [[ ${#PKG_NAMES[@]} -eq 0 ]]; then
    echo "No ROS 2 packages found in ${BASE_DIR}"
    exit 1
fi

echo "Found packages: ${PKG_NAMES[*]}"

cd "${BASE_DIR}"
colcon build \
    --packages-select "${PKG_NAMES[@]}" \
    --merge-install \
    --install-base "${INSTALL_PREFIX}"

echo "Installed ${#PKG_NAMES[@]} ROS2 packages to ${INSTALL_PREFIX}/share"
