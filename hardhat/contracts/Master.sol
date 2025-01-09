pragma solidity ^0.8.19;
import "@chainlink/contracts/src/v0.8/automation/interfaces/AutomationCompatibleInterface.sol"; //chainlink automation interface

contract TwoPhaseCommit is AutomationCompatibleInterface {
    uint256 public dataCounter;

    struct StoredData {
        bytes encryptedData;
        string owner;
        string dataName;
        uint256 releaseTime;
        bool keyReleased;
        int releasePhase;
        uint256 dataId;
    }

    struct PrivateKeys {
        string owner;
        string dataName;
        bytes privateKey;
        uint256 dataId;
    }
    
    StoredData[] public storedData;
    //mapping
    PrivateKeys[] private privateKeys; //?? store as private???

    
    event ReleaseEncryptedData(
        bytes encryptedData,
        string owner,
        string dataName,
        uint256 releaseTime
    );


    event KeyReleaseRequested(uint256 index, string owner, string dataName);
    event KeyReleased(bytes privateKey, string owner, string dataName, uint256 dataId);

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
    
    function addStoredData(
        bytes memory _encryptedData,
        string memory _owner,
        string memory _dataName,
        uint256 _releaseTime
    ) 
        public     
        ValidInput(_encryptedData, _owner, _dataName, _releaseTime) 
        {  
        dataCounter++; //increment data counter for each new data added
        storedData.push(StoredData({
            encryptedData: _encryptedData,
            owner: _owner,
            dataName: _dataName,
            releaseTime: _releaseTime,
            keyReleased: false,
            releasePhase: 0,
            dataId: dataCounter 
        }));

        //send out the encrypted data immediately 
        emit ReleaseEncryptedData(_encryptedData, _owner, _dataName, _releaseTime);
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
                emit KeyReleaseRequested(i, storedData[i].owner, storedData[i].dataName);
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
