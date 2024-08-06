#!/bin/bash

set -e

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
    local architectures=$6
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
                "architectures": $architectures
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
    local architectures=$6
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
                "architectures": $architectures
            }
        }
    },
EOF
}

# Processes the intermediate image list sorting by creation date
function process_intermediate_list() {
    local version=$1
    local file=$2
    local type=$3
    local limit=$4
    shift 4

    local IFS=$'\n'
    local sorted_list=($(echo "$@" | jq -nc '[inputs]' | jq '. |= sort_by(.created) | reverse' | jq -c '.[]'))

    echo "Limiting $limit entries for version $version:"

    for ((i = 0; i < ${#sorted_list[@]} && i < $limit; i++)); do
        local entry="${sorted_list[$i]}"
        
        echo -e "- $(( i + 1 )): $entry"

        local image_uri=$(echo "$entry" | jq '.uri' | sed 's/"//g')
        local version=$(echo "$entry" | jq '.version' | sed 's/"//g')
        local managed_os_version_name=$(echo "$entry" | jq '.managedOSVersionName' | sed 's/"//g')
        local display_name=$(echo "$entry" | jq '.displayName' | sed 's/"//g')
        local architectures=$(echo "$entry" | jq -c '[.architectures[]]')

        if [[ "$type" == "os" ]]; then
            append_os_entry "$file" "$managed_os_version_name" "$version" "$image_uri" "$display_name" "$architectures"
        elif [[ "$type" == "iso" ]]; then
            append_iso_entry "$file" "$managed_os_version_name" "$version" "$image_uri" "$display_name" "$architectures"
        fi
    done
}

# Processes an entire repository and creates a list of all images
function process_repo() {
    local repo=$1
    local repo_type=$2
    local file=$3
    local limit=$4
    local flavor=$5
    local display_name=$6

    local intermediate_list=()
    local tags=($(skopeo list-tags docker://$repo | jq '.Tags[]' | grep -v '.att\|.sig\|latest' | sed 's/"//g'))
    
    echo "Processing repository: $repo"

    for tag in "${tags[@]}"; do
        # Version (non-build) tag (ex '1.2.3')
        if [[ $tag =~ ^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$ ]]; then
            # If the intermediate_list is not empty, 
            # it means we are done processing the previously met version tag.
            if [[ -n $intermediate_list ]]; then
                process_intermediate_list "$processing_version" "$file" "$repo_type" $limit "${intermediate_list[@]}"
                local intermediate_list=()
            fi
            local processing_version="$tag"
            continue
        fi
        local image_uri="$repo:$tag"
        local image_creation_date=($(skopeo inspect docker://$image_uri | jq '.Created' | sed 's/"//g'))
        local raw_inspect_output=$(skopeo inspect --raw docker://$image_uri)
        # If there is no list of architectures, assume only the one that runs this script is available.
        local architectures="[\"amd64\"]"
        if echo "$raw_inspect_output" | jq '.manifests | length > 0' | grep "true"; then
            architectures=$(echo "$raw_inspect_output" | jq -c '[.manifests[].platform.architecture]')
        fi
        local managed_os_version_name=$(format_managed_os_version_name "$flavor" "$tag" "$repo_type")
        # Append entry to intermediate list
        local intermediate_entry="{\"uri\":\"$image_uri\",\"created\":\"$image_creation_date\",\"version\":\"$tag\",\"managedOSVersionName\":\"$managed_os_version_name\",\"displayName\":\"$display_name\",\"architectures\":$architectures}"
        echo "Intermediate: $intermediate_entry"
        local intermediate_list=("${intermediate_list[@]}" "$intermediate_entry")
    done
    # Process the intermediate_list again for the last remaining version
    if [[ -n $intermediate_list ]]; then
        process_intermediate_list "$processing_version" "$file" "$repo_type" $limit "${intermediate_list[@]}"
    fi
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

    # Start writing the channel file by opening a JSON array
    file="channels/$file_name.json"
    echo "Creating $file_name"
    echo "[" > $file

    # Process OS container tags
    process_repo "$os_repo" "os" "$file" "$limit" "$flavor" "$display_name"

    # Process ISO container tags (if applicable)
    if [ "$iso_repo" != "N/A" ]; then
        process_repo "$iso_repo" "iso" "$file" "$limit" "$flavor" "$display_name"
    fi

    # Delete trailing ',' from array. (technically last written char on the file)
    sed -i '$ s/.$//' $file

    # Close the JSON Array
    echo "]" >> $file

    # Validate the JSON file
    cat $file | jq empty
done <<END
$watches
END
