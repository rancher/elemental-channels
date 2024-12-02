#!/bin/bash

set -e

# Lists all tags available in a repository
function list_tags() {
   local repo=$1

   skopeo list-tags "docker://${repo}" | jq -r '.Tags[]'
}

# Lists all minor versions available in repository
function list_minor_versions() {
    local repo=$1

    list_tags "${repo}" | grep -oE "^[0-9]+\.[0-9]+" | sort -rV | uniq
}

# Lists all versions available in repo matching a minor version
function list_versions_matching_minor() {
    local repo=$1
    local minor=$2

    # Assumes the pattern of MAJOR.MINOR.PATCH-X.Y, where X and Y are the commit count and build count in OBS
    list_tags "${repo}" | grep -E "^[0-9]+\.[0-9]+\.[0-9]+-[0-9]+\.[0-9]+" | grep -E "^${minor}" | sort -rV | uniq
}

# is_higher_version returns 0 if version A i higher than version B, returns 1 otherwise
function is_higher_version() {
    local versionA=$1
    local versionB=$2

    # Note sort -V is a natural numbering sort, not semver.
    # So semver pre-releses should NOT be compared with this method
    higher_ver=$(printf '%s\n' "${versionA}" "${versionB}" | sort -rV | head -n1)
    [ "${higher_ver}" == "${versionA}" ] && return 0
    return 1
} 

# Prefixes the ManagedOSVersion name with flavor value, if any
function format_managed_os_version_name() {
    local flavor=$1
    local tag=$2
    local type=$3
    if [ -z "$flavor" ]; then
        echo "v${tag}-${type}"
    else
        echo "${flavor}-v${tag}-${type}"
    fi
}

# Prints one OS JSON array entry
function append_os_entry() {
    local file=$1
    local os_version_name=$2
    local version=$3
    local image_uri=$4
    local display_name=$5
    local platforms=$6
    local creation_date_epoch=$7
    cat >> "$file" << EOF
    {
        "metadata": {
            "name": "$os_version_name"
        },
        "spec": {
            "version": "v$version",
            "type": "container",
            "metadata": {
                "upgradeImage": "$image_uri",
                "displayName": "$display_name OS",
                "platforms": $platforms,
                "created": $creation_date_epoch
            }
        }
    },
EOF
}

# Prints one ISO JSON array entry
function append_iso_entry() {
    local file=$1
    local os_version_name=$2
    local version=$3
    local image_uri=$4
    local display_name=$5
    local platforms=$6
    local creation_date_epoch=$7
    cat >> "$file" << EOF
    {
        "metadata": {
            "name": "$os_version_name"
        },
        "spec": {
            "version": "v$version",
            "type": "iso",
            "metadata": {
                "uri": "$image_uri",
                "displayName": "$display_name ISO",
                "platforms": $platforms,
                "created": $creation_date_epoch
            }
        }
    },
EOF
}

# Processes the intermediate image list
function process_intermediate_list() {
    local version=$1
    local file=$2
    local type=$3
    local limit=$4
    shift 4

    for entry in "$@"; do
        local image_uri=$(echo "$entry" | jq -r '.uri')
        local version=$(echo "$entry" | jq -r '.version')
        local managed_os_version_name=$(echo "$entry" | jq -r '.managedOSVersionName')
        local display_name=$(echo "$entry" | jq -r '.displayName')
        local platforms=$(echo "$entry" | jq -c '[.platforms[]]')
        local creation_date=$(echo "$entry" | jq -r '.created')

        local creation_date_epoch=$(date -d "$creation_date" +"%s")

        if [[ "$type" == "os" ]]; then
            append_os_entry "$file" "$managed_os_version_name" "$version" "$image_uri" "$display_name" "$platforms" "$creation_date_epoch"
        elif [[ "$type" == "iso" ]]; then
            append_iso_entry "$file" "$managed_os_version_name" "$version" "$image_uri" "$display_name" "$platforms" "$creation_date_epoch"
        fi
    done
}

# Processes an entire repository and creates a list of images
function process_repo() {
    local repo=$1
    local repo_type=$2
    local file=$3
    local limit=$4
    local flavor=$5
    local display_name=$6
    local min_version=$7

    for minor_version in $(list_minor_versions "${repo}"); do
        local intermediate_list=()
        local img_count=0
        for version in $(list_versions_matching_minor "${repo}" "${minor_version}"); do
	    # Ignore from this version and on, does not match the minimum criteria
            is_higher_version "${min_version}" "${version}" && break

	    # Limit the mount of images listed
	    [ "${img_count}" -ge "${limit}" ] && break

	    local image_uri="${repo}:${version}"
            local image_creation_date=($(skopeo inspect docker://$image_uri | jq '.Created' | sed 's/"//g'))
	    local raw_inspect_output=$(skopeo inspect --raw "docker://${image_uri}")
            # If there is no list of platforms, assume only the one that runs this script is available.
            local platforms="[\"amd64\"]"
            if echo "$raw_inspect_output" | jq '.manifests | length > 0' | grep "true" > /dev/null; then
                platforms=$(echo "$raw_inspect_output" | jq -c '[.manifests[].platform.architecture]')
            fi
            platforms="${platforms/amd64/linux\/x86_64}"
            platforms="${platforms/arm64/linux\/aarch64}"
            local managed_os_version_name=$(format_managed_os_version_name "$flavor" "$version" "$repo_type")
            # Append entry to intermediate list
            local intermediate_entry="{\"uri\":\"$image_uri\",\"created\":\"$image_creation_date\",\"version\":\"$version\",\"managedOSVersionName\":\"$managed_os_version_name\",\"displayName\":\"$display_name\",\"platforms\":$platforms}"
            echo "Intermediate: $intermediate_entry"
            local intermediate_list=("${intermediate_list[@]}" "$intermediate_entry")
	    if [[ "${repo_type}" ==  "os" ]]; then
                podman pull "${image_uri}"
                podman run --rm "${image_uri}" rpm -qa --qf "%{NAME}|%{EPOCH}|%{VERSION}|%{RELEASE}|%{ARCH}|%{DISTURL}|%{LICENSE}\n" | sort > "channels/${managed_os_version_name}.packages"
		podman rmi "${image_uri}"
	    fi

            img_count=$((img_count + 1))
        done
        if [[ -n $intermediate_list ]]; then
            process_intermediate_list "${minor_version}" "$file" "$repo_type" $limit "${intermediate_list[@]}"
        fi
    done
}


# The list of repositories to watch
watches=$(yq e -o=j -I=0 '.watches[]' config.yaml)

# Loop through all watches
while IFS=\= read watch; do
    # Parse one entry
    flavor=$(echo "$watch" | yq e '.flavor')
    file_name=$(echo "$watch" | yq e '.fileName')
    display_name=$(echo "$watch" | yq e '.displayName')
    os_repo=$(echo "$watch" | yq e '.osRepo')
    iso_repo=$(echo "$watch" | yq e '.isoRepo')
    limit=$(echo "$watch" | yq e '.limit')
    # Allow all versions if min_version is not set
    min_version=$(echo "$watch" | yq e -e '.minVersion' 2> /dev/null) || min_version="0.0.0"

    # Start writing the channel file by opening a JSON array
    file="channels/$file_name.json"
    echo "Creating $file_name"
    echo "[" > $file

    # Process OS container tags
    process_repo "$os_repo" "os" "$file" "$limit" "$flavor" "$display_name" "${min_version}"

    # Process ISO container tags (if applicable)
    if [ "$iso_repo" != "N/A" ]; then
        process_repo "$iso_repo" "iso" "$file" "$limit" "$flavor" "$display_name" "${min_version}"
    fi

    # Delete trailing ',' from array. (technically last written char on the file)
    sed -i '$ s/.$//' $file

    # Close the JSON Array
    echo "]" >> $file

    # Validate the JSON file
    cat $file | jq empty

    # Create tarball for packages list of ech image in the channel
    rm -f "channels/${file_name}.packages.tar"
    while IFS= read -r -d '' pkgs_file; do
        tar --mtime="@0" --owner=0 --group=0 --pax-option=exthdr.name=%d/PaxHeaders/%f,delete=atime,delete=ctime \
            -C channels -rf "channels/${file_name}.packages.tar" "${pkgs_file##channels/}"
        rm "${pkgs_file}"
    done < <(find channels/ -mindepth 1 -maxdepth 1 -type f -name "*.packages" -print0)
done <<END
$watches
END
