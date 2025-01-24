// SPDX-License-Identifier: MIT

pragma solidity ^0.8.19;
import "@chainlink/contracts/src/v0.8/automation/interfaces/AutomationCompatibleInterface.sol"; //chainlink automation interface

contract TwoPhaseDissemination is AutomationCompatibleInterface {
    uint256 public dataCounter;

    struct VectorClock {
        address process;
        uint256 timeStamp;
    }

    struct StoredData {
        bytes encryptedData;
        string owner;
        string dataName;
        uint256 releaseTime;
        bool keyReleased;
        int releasePhase;
        bytes privateKey;
        uint256 dataId;
        VectorClock[] vectorClocks;
        bytes32[] dependencies;
        uint256 securityLevel;
    }

    struct PrivateKeys {
        string owner;
        string dataName;
        bytes privateKey;
        uint256 dataId;
    }
    
    StoredData[] public storedData;
    //mapping
    PrivateKeys[] private privateKeys; 

    mapping(address => uint256) public processSecurityLevel;
    mapping(string => uint256) public dataSecurityLevel;

    
    event ReleaseEncryptedData(
        bytes encryptedData,
        bytes privateKey,
        string owner,
        string dataName,
        uint256 releaseTime,
        uint256 dataId,
        VectorClock[] vectorClocks,
        bytes32[] dependencies
    );

    event DependencyCheck(bytes32 dependency, bytes32 calculatedHash, bool found);
    event KeyReleaseRequested(uint256 index, string owner, string dataName, uint256 dataId);
    event KeyReleased(bytes privateKey, string owner, string dataName, uint256 dataId);

    function setProcessSecurityLevel(address _process, uint256 level) public {
        require(level > 0, "Security level must be greater than 0");
        processSecurityLevel[_process] = level;
    }

    function setDataSecurityLevel(string memory _dataName, uint256 level) public {
        require(level > 0, "Security level must be greater than 0");
        dataSecurityLevel[_dataName] = level;
    }

    function getCurrentDataId() public view returns (uint256) {
        return dataCounter;
    }

    //get all stored data that is > the dataId
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

    function getDependencyReleaseTimes(bytes32[] memory dependencies) public view returns (uint256[] memory){
        uint256[] memory releaseTimes = new uint256[](dependencies.length);

        for (uint i = 0; i < dependencies.length; i++){
            releaseTimes[i] = 0;
            for (uint j = 0; j < storedData.length; j++){
                if (keccak256(abi.encodePacked(storedData[j].encryptedData)) == dependencies[i]){
                    releaseTimes[i] = storedData[j].releaseTime;
                    break;
                }
            }
        }
        return releaseTimes;
    } 


    function addStoredData(
        bytes memory _encryptedData,
        bytes memory _privateKey,
        string memory _owner,
        string memory _dataName,
        uint256 _releaseTime,
        bytes32[] memory _dependencies,
        uint256 _securityLevel
    ) 
        public     
        ValidInput(_encryptedData, _owner, _dataName, _releaseTime) 
        ValidSecurityLevel(_securityLevel)
    {  
        require(processSecurityLevel[msg.sender] >= _securityLevel, "Process security level is too low");

        for (uint i = 0; i < _dependencies.length; i++) {
            bool found = false; 
            for (uint j = 0; j < storedData.length; j++){
                if (keccak256(abi.encodePacked(storedData[j].encryptedData)) == _dependencies[i]){
                    require(storedData[j].releaseTime <= _releaseTime, "Release time must be after all dependencies");
                    found = true;
                    break;
                }
            }
            require(found, "Dependency not found");
        }
        dataCounter++; //increment data counter for each new data added

        StoredData storage newData = storedData.push();
        newData.encryptedData = _encryptedData;
        newData.privateKey = _privateKey;
        newData.owner = _owner;
        newData.dataName = _dataName;
        newData.releaseTime = _releaseTime;
        newData.keyReleased = false;
        newData.releasePhase = 0;
        newData.dataId = dataCounter;
        newData.dependencies = _dependencies;
        newData.securityLevel = _securityLevel;

        // Create vector clock directly in storage
        newData.vectorClocks.push(VectorClock({
            process: msg.sender,
            timeStamp: block.timestamp
        }));

        //send out the encrypted data immediately 
        emit ReleaseEncryptedData(
            _encryptedData, 
            _privateKey,
            _owner, 
            _dataName, 
            _releaseTime, 
            dataCounter, 
            newData.vectorClocks, 
            _dependencies
        );
    }

function checkMessageDependencies(bytes32[] memory dependencies) internal view returns (bool) {
    for (uint i = 0; i < dependencies.length; i++) {
        bool found = false;
        for (uint j = 0; j < storedData.length; j++) {
            bytes32 calculatedHash = keccak256(abi.encodePacked(storedData[j].encryptedData));
            if (calculatedHash == dependencies[i]) {
                // Also check if the key has been released
                if (!storedData[j].keyReleased) {
                    return false;  // Dependency exists but key not released
                }
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

// Separate non-view debug function that can emit events if needed
function debugCheckDependency(bytes32 dependency) public returns(bool, bytes32[] memory) {
    bytes32[] memory dependencies = new bytes32[](1);
    dependencies[0] = dependency;

    bytes32[] memory calculatedHashes = new bytes32[](storedData.length);
    for (uint i = 0; i < storedData.length; i++) {
        calculatedHashes[i] = keccak256(abi.encodePacked(storedData[i].encryptedData));
        emit DependencyCheck(dependency, calculatedHashes[i], calculatedHashes[i] == dependency);
    }
    return (checkMessageDependencies(dependencies), calculatedHashes);
}

// Keep the view version for normal calls
function checkDependency(bytes32 dependency) public view returns(bool, bytes32[] memory) {
    bytes32[] memory dependencies = new bytes32[](1);
    dependencies[0] = dependency;

    bytes32[] memory calculatedHashes = new bytes32[](storedData.length);
    for (uint i = 0; i < storedData.length; i++) {
        calculatedHashes[i] = keccak256(abi.encodePacked(storedData[i].encryptedData));
    }
    return (checkMessageDependencies(dependencies), calculatedHashes);
}



    function checkUpkeep(bytes calldata )
        external
        view
        override
        returns (bool upkeepNeeded, bytes memory performData)
    {
        upkeepNeeded = false;
        for (uint i = 0; i < storedData.length; i++) {
            if (storedData[i].releasePhase == 0 && storedData[i].releaseTime <= block.timestamp) {
                upkeepNeeded = true;
                break;
            }
        }
        return (upkeepNeeded, "");
    }

    function performUpkeep(bytes calldata ) external override {
        for (uint i = 0; i < storedData.length; i++) {
            if (storedData[i].releasePhase == 0 && storedData[i].releaseTime <= block.timestamp) {
                storedData[i].releasePhase = 1;
                emit KeyReleaseRequested(i, storedData[i].owner, storedData[i].dataName, storedData[i].dataId);
            }
        }
    }
    

    function releaseKey(string memory _dataName, string memory _owner, bytes memory _privateKey) public { //will the private key sumbission be public???
        for (uint i = 0; i < storedData.length; i++) {
            if (
                keccak256(abi.encodePacked(storedData[i].dataName)) == keccak256(abi.encodePacked(_dataName)) &&
                keccak256(abi.encodePacked(storedData[i].owner)) == keccak256(abi.encodePacked(_owner))
            ) {
                require(!storedData[i].keyReleased, "Key already released");
                require (processSecurityLevel[msg.sender] >= storedData[i].securityLevel, "Process security level is too low");
                require(checkMessageDependencies(storedData[i].dependencies), "Not all dependenices met");
                storedData[i].keyReleased = true;
                privateKeys.push(PrivateKeys({
                    owner: _owner,
                    dataName: _dataName,
                    privateKey: _privateKey,
                    dataId: storedData[i].dataId
                }));
                emit KeyReleased(_privateKey, _owner, _dataName, storedData[i].dataId);
                break;
            }
        }
    }


    modifier ValidSecurityLevel(uint256 _securityLevel) {
        require(_securityLevel > 0, "Security level must be greater than 0");
        _;
    }
    
    modifier ValidInput(
        bytes memory _encryptedData,
        string memory _owner,
        string memory _dataName,
        uint256 _releaseTime
    ) {
        require(_encryptedData.length > 0, "Encrypted data is required");
        require(bytes(_owner).length > 0, "Owner is required");
        require(bytes(_dataName).length > 0, "Data name is required");
        require(_releaseTime > block.timestamp, "Release time must be in the future");
        _;
    }
}
