// SPDX-License-Identifier: MIT

pragma solidity ^0.8.19;

contract LamportClock {
    uint256 public dataCounter;

    struct VectorClock {
        address process;
        uint256 timeStamp;
    }

    struct StoredData {
        bytes data;
        string owner;
        string dataName;
        uint256 messageTimestamp;
        uint256 dataId;
        VectorClock[] vectorClocks;
        bytes32[] dependencies;
    }

    StoredData[] public storedData;
    
    // Mapping to quickly check if a message with a given hash exists
    // This optimizes dependency checking
    mapping(bytes32 => bool) public messageExists;
    mapping(bytes32 => uint256) public messageTimestamps;
    
    event BroadcastMessage(
        bytes data,
        string owner,
        string dataName,
        uint256 messageTimestamp,
        uint256 dataId,
        VectorClock[] vectorClocks,
        bytes32[] dependencies
    );

    event DependencyCheck(bytes32 dependency, bytes32 calculatedHash, bool found);

    function getCurrentDataId() public view returns (uint256) {
        return dataCounter;
    }

    function getMissingDataItems(uint256 _dataId) public view returns (StoredData[] memory) {
        require(_dataId < dataCounter, "Data ID is invalid");
        uint256 itemCount = 0;
        for (uint256 i = _dataId; i < dataCounter; i++) {
            itemCount++;
        }
        
        StoredData[] memory missingData = new StoredData[](itemCount);
        uint256 j = 0; 
        for (uint256 i = _dataId; i < dataCounter; i++) {
            missingData[j] = storedData[i];
            j++;
        }
        return missingData;
    }

    function getDependencyTimestamps(bytes32[] memory dependencies) public view returns (uint256[] memory) {
        uint256[] memory timestamps = new uint256[](dependencies.length);

        for (uint i = 0; i < dependencies.length; i++) {
            timestamps[i] = messageTimestamps[dependencies[i]];
        }
        return timestamps;
    }

    function publishMessage(
        bytes memory _data,
        string memory _owner,
        string memory _dataName,
        bytes32[] memory _dependencies
    ) 
        public     
        ValidInput(_data, _owner, _dataName) 
    {  
        // Validate dependencies using the optimized mapping
        for (uint i = 0; i < _dependencies.length; i++) {
            require(messageExists[_dependencies[i]], "Dependency not found");
        }
        
        dataCounter++;

        StoredData storage newData = storedData.push();
        newData.data = _data;
        newData.owner = _owner;
        newData.dataName = _dataName;
        newData.messageTimestamp = block.timestamp;
        newData.dataId = dataCounter;
        newData.dependencies = _dependencies;

        // Create vector clock directly in storage
        newData.vectorClocks.push(VectorClock({
            process: msg.sender,
            timeStamp: block.timestamp
        }));

        // Store message hash for quick lookup
        bytes32 msgHash = keccak256(abi.encodePacked(_data));
        messageExists[msgHash] = true;
        messageTimestamps[msgHash] = block.timestamp;

        // Broadcast the message immediately
        emit BroadcastMessage(
            _data, 
            _owner, 
            _dataName, 
            block.timestamp, 
            dataCounter, 
            newData.vectorClocks, 
            _dependencies
        );
    }

    // New batch publishing function to handle multiple messages at once
    function publishBatchMessages(
        bytes[] memory _dataArray,
        string[] memory _ownerArray,
        string[] memory _dataNameArray,
        bytes32[][] memory _dependenciesArray
    ) public {
        // Validate input arrays have the same length
        require(_dataArray.length == _ownerArray.length, "Arrays must have same length");
        require(_dataArray.length == _dataNameArray.length, "Arrays must have same length");
        require(_dataArray.length == _dependenciesArray.length, "Arrays must have same length");
        
        // Process each message in the batch
        for (uint i = 0; i < _dataArray.length; i++) {
            // Skip validation for empty data
            if (_dataArray[i].length == 0 || bytes(_ownerArray[i]).length == 0 || bytes(_dataNameArray[i]).length == 0) {
                continue;
            }
            
            // Validate dependencies
            bool dependenciesValid = true;
            for (uint j = 0; j < _dependenciesArray[i].length; j++) {
                if (!messageExists[_dependenciesArray[i][j]]) {
                    dependenciesValid = false;
                    break;
                }
            }
            
            // Skip this message if dependencies aren't valid
            if (!dependenciesValid) {
                continue;
            }
            
            // Process this message
            dataCounter++;
            
            StoredData storage newData = storedData.push();
            newData.data = _dataArray[i];
            newData.owner = _ownerArray[i];
            newData.dataName = _dataNameArray[i];
            newData.messageTimestamp = block.timestamp;
            newData.dataId = dataCounter;
            newData.dependencies = _dependenciesArray[i];
            
            // Create vector clock
            newData.vectorClocks.push(VectorClock({
                process: msg.sender,
                timeStamp: block.timestamp
            }));
            
            // Store message hash for quick lookup
            bytes32 msgHash = keccak256(abi.encodePacked(_dataArray[i]));
            messageExists[msgHash] = true;
            messageTimestamps[msgHash] = block.timestamp;
            
            // Emit event for this message
            emit BroadcastMessage(
                _dataArray[i],
                _ownerArray[i],
                _dataNameArray[i],
                block.timestamp,
                dataCounter,
                newData.vectorClocks,
                _dependenciesArray[i]
            );
        }
    }
    
    function checkMessageDependencies(bytes32[] memory dependencies) internal view returns (bool) {
        for (uint i = 0; i < dependencies.length; i++) {
            if (!messageExists[dependencies[i]]) {
                return false;  // Dependency not found
            }
        }
        return true;
    }

    function debugCheckDependency(bytes32 dependency) public returns(bool, bytes32[] memory) {
        bytes32[] memory dependencies = new bytes32[](1);
        dependencies[0] = dependency;

        bytes32[] memory calculatedHashes = new bytes32[](storedData.length);
        for (uint i = 0; i < storedData.length; i++) {
            calculatedHashes[i] = keccak256(abi.encodePacked(storedData[i].data));
            emit DependencyCheck(dependency, calculatedHashes[i], calculatedHashes[i] == dependency);
        }
        return (checkMessageDependencies(dependencies), calculatedHashes);
    }

    function checkDependency(bytes32 dependency) public view returns(bool, bytes32[] memory) {
        bytes32[] memory dependencies = new bytes32[](1);
        dependencies[0] = dependency;

        bytes32[] memory calculatedHashes = new bytes32[](storedData.length);
        for (uint i = 0; i < storedData.length; i++) {
            calculatedHashes[i] = keccak256(abi.encodePacked(storedData[i].data));
        }
        return (checkMessageDependencies(dependencies), calculatedHashes);
    }
    
    modifier ValidInput(
        bytes memory _data,
        string memory _owner,
        string memory _dataName
    ) {
        require(_data.length > 0, "Data is required");
        require(bytes(_owner).length > 0, "Owner is required");
        require(bytes(_dataName).length > 0, "Data name is required");
        _;
    }
}
