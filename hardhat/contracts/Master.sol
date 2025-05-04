// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

contract LamportClock {
    uint256 public dataCounter;

    struct StoredData {
        bytes    data;
        string   owner;
        string   dataName;
        uint256  messageTimestamp;
        uint256  dataId;
        bytes32[] dependencies;
    }

    StoredData[] public storedData;
    mapping(bytes32 => bool)    public messageExists;
    mapping(bytes32 => uint256) public messageTimestamps;

    event FastBroadcast(bytes32 indexed dataHash, uint256 indexed dataId);

    /* ─── views ───────────────────────────────────── */
    function getCurrentDataId() external view returns (uint256) {
        return dataCounter;
    }

    function getStoredData(uint256 id)
        external view returns (StoredData memory)
    {
        require(id > 0 && id <= dataCounter, "invalid id");
        return storedData[id - 1];
    }

    function getMissingDataItems(uint256 fromId)
        external view returns (StoredData[] memory out)
    {
        require(fromId < dataCounter, "id too high");
        uint256 n = dataCounter - fromId;
        out = new StoredData[](n);
        for (uint256 i; i < n; ++i) out[i] = storedData[fromId + i];
    }

      function getDependencyTimestamps(bytes32[] memory dependencies) public view returns (uint256[] memory) {
        uint256[] memory timestamps = new uint256[](dependencies.length);

        for (uint i = 0; i < dependencies.length; i++) {
            timestamps[i] = messageTimestamps[dependencies[i]];
        }
        return timestamps;
    }

    /* ─── publish ─────────────────────────────────── */
    function publishMessage(
        bytes calldata   _data,
        string calldata  _owner,
        string calldata  _dataName,
        bytes32[] calldata _deps
    )
        external
    {
        for (uint256 i; i < _deps.length; ++i)
            require(messageExists[_deps[i]], "dep missing");

        dataCounter += 1;
        StoredData storage s = storedData.push();
        s.data              = _data;
        s.owner             = _owner;
        s.dataName          = _dataName;
        s.messageTimestamp  = block.timestamp;
        s.dataId            = dataCounter;
        s.dependencies      = _deps;

        bytes32 h = keccak256(_data);
        messageExists[h]     = true;
        messageTimestamps[h] = block.timestamp;

        emit FastBroadcast(h, dataCounter);
    }
}

