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
            timestamps[i] = 0;
            for (uint j = 0; j < storedData.length; j++) {
                if (keccak256(abi.encodePacked(storedData[j].data)) == dependencies[i]) {
                    timestamps[i] = storedData[j].messageTimestamp;
                    break;
                }
            }
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
        // Validate dependencies
        for (uint i = 0; i < _dependencies.length; i++) {
            bool found = false; 
            for (uint j = 0; j < storedData.length; j++) {
                if (keccak256(abi.encodePacked(storedData[j].data)) == _dependencies[i]) {
                    found = true;
                    break;
                }
            }
            require(found, "Dependency not found");
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

    function checkMessageDependencies(bytes32[] memory dependencies) internal view returns (bool) {
        for (uint i = 0; i < dependencies.length; i++) {
            bool found = false;
            for (uint j = 0; j < storedData.length; j++) {
                bytes32 calculatedHash = keccak256(abi.encodePacked(storedData[j].data));
                if (calculatedHash == dependencies[i]) {
                    found = true;
                    break;
                }
            }
            if (!found) {
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
